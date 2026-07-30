// Package admin contains administrator-only application services.
package admin

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidInput = errors.New("invalid admin query")
	ErrUserNotFound = errors.New("user not found")
	ErrPlanNotFound = errors.New("plan not found")
)

const (
	defaultPageSize = 50
	maxPageSize     = 100
)

type Service struct{ db *sql.DB }

type SortField string

const (
	SortByTransfer SortField = "transfer"
	SortByMinutes  SortField = "minutes"
	SortByUsername SortField = "username"
	SortByCreated  SortField = "created_at"
)

type SortDirection string

const (
	Ascending  SortDirection = "asc"
	Descending SortDirection = "desc"
)

type ListUsersInput struct {
	Search    string
	Limit     int
	Cursor    string
	Sort      SortField
	Direction SortDirection
}

type User struct {
	ID                   string     `json:"id"`
	Username             string     `json:"username"`
	Email                string     `json:"email"`
	Plan                 string     `json:"plan"`
	IsAdmin              bool       `json:"is_admin"`
	MonthlyMinutesUsed   int64      `json:"monthly_minutes_used"`
	MonthlyTransferBytes int64      `json:"monthly_transfer_bytes_used"`
	CreatedAt            time.Time  `json:"created_at"`
	LastLoginAt          *time.Time `json:"last_login_at"`
}

type UserPage struct {
	Users      []User `json:"users"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type cursor struct {
	Value string `json:"value"`
	ID    string `json:"id"`
}

func NewService(db *sql.DB) *Service { return &Service{db: db} }

func (s *Service) ListUsers(ctx context.Context, input ListUsersInput) (UserPage, error) {
	if s == nil || s.db == nil {
		return UserPage{}, errors.New("admin database unavailable")
	}

	limit := input.Limit
	if limit == 0 {
		limit = defaultPageSize
	}
	if limit < 1 || limit > maxPageSize {
		return UserPage{}, ErrInvalidInput
	}

	sortField := input.Sort
	if sortField == "" {
		sortField = SortByCreated
	}
	if sortField != SortByTransfer && sortField != SortByMinutes && sortField != SortByUsername && sortField != SortByCreated {
		return UserPage{}, ErrInvalidInput
	}

	direction := input.Direction
	if direction == "" {
		direction = Descending
		if sortField == SortByUsername {
			direction = Ascending
		}
	}
	if direction != Ascending && direction != Descending {
		return UserPage{}, ErrInvalidInput
	}

	cursorValue, cursorID, err := decodeCursor(input.Cursor)
	if err != nil {
		return UserPage{}, err
	}

	query := `
		SELECT users.id, users.username, users.email, plans.name, users.is_admin,
			users.monthly_minutes_used, users.monthly_transfer_bytes_used,
			users.created_at, users.last_login_at
		FROM users
		JOIN plans ON plans.id = users.plan_id`
	args := make([]any, 0, 5)
	where := make([]string, 0, 2)

	search := escapeLike(strings.ToLower(strings.TrimSpace(input.Search)))
	if search != "" {
		args = append(args, search+"%")
		placeholder := len(args)
		where = append(where, fmt.Sprintf("(users.username LIKE $%d ESCAPE '\\' OR users.email LIKE $%d ESCAPE '\\')", placeholder, placeholder))
	}

	orderColumn := map[SortField]string{
		SortByTransfer: "users.monthly_transfer_bytes_used",
		SortByMinutes:  "users.monthly_minutes_used",
		SortByUsername: "users.username",
		SortByCreated:  "users.created_at",
	}[sortField]
	if cursorValue != "" {
		cursorArg, err := typedCursorValue(cursorValue, sortField)
		if err != nil {
			return UserPage{}, ErrInvalidInput
		}
		args = append(args, cursorArg, cursorID)
		valueArg, idArg := len(args)-1, len(args)
		comparison := ">"
		if direction == Descending {
			comparison = "<"
		}
		where = append(where, fmt.Sprintf("(%s %s $%d OR (%s = $%d AND users.id > $%d))", orderColumn, comparison, valueArg, orderColumn, valueArg, idArg))
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += fmt.Sprintf(" ORDER BY %s %s, users.id ASC LIMIT $%d", orderColumn, direction, len(args)+1)
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return UserPage{}, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	page := UserPage{Users: make([]User, 0, limit)}
	for rows.Next() {
		var account User
		if err := rows.Scan(
			&account.ID, &account.Username, &account.Email, &account.Plan, &account.IsAdmin,
			&account.MonthlyMinutesUsed, &account.MonthlyTransferBytes,
			&account.CreatedAt, &account.LastLoginAt,
		); err != nil {
			return UserPage{}, fmt.Errorf("scan user: %w", err)
		}
		page.Users = append(page.Users, account)
	}
	if err := rows.Err(); err != nil {
		return UserPage{}, fmt.Errorf("iterate users: %w", err)
	}
	if len(page.Users) > limit {
		page.Users = page.Users[:limit]
		last := page.Users[len(page.Users)-1]
		page.NextCursor = encodeCursor(sortValue(last, sortField), last.ID)
	}
	return page, nil
}

// ChangeUserPlan assigns an existing plan to a user and returns the updated
// user summary. Usage counters are intentionally preserved.
func (s *Service) ChangeUserPlan(ctx context.Context, userID string, planID int64) (User, error) {
	if s == nil || s.db == nil {
		return User{}, errors.New("admin database unavailable")
	}
	if strings.TrimSpace(userID) == "" || planID < 1 {
		return User{}, ErrInvalidInput
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, fmt.Errorf("begin change user plan: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var existingPlanID int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM plans WHERE id = $1", planID).Scan(&existingPlanID); errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrPlanNotFound
	} else if err != nil {
		return User{}, fmt.Errorf("find plan: %w", err)
	}

	var account User
	err = tx.QueryRowContext(ctx, `
		UPDATE users
		SET plan_id = $1
		WHERE id = $2
		RETURNING users.id, users.username, users.email,
			(SELECT plans.name FROM plans WHERE plans.id = users.plan_id), users.is_admin,
			users.monthly_minutes_used, users.monthly_transfer_bytes_used,
			users.created_at, users.last_login_at`, planID, userID).Scan(
		&account.ID, &account.Username, &account.Email, &account.Plan, &account.IsAdmin,
		&account.MonthlyMinutesUsed, &account.MonthlyTransferBytes,
		&account.CreatedAt, &account.LastLoginAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("change user plan: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return User{}, fmt.Errorf("commit user plan change: %w", err)
	}
	return account, nil
}

// SetUserAdmin updates a user's administrator permission and returns the
// updated user summary.
func (s *Service) SetUserAdmin(ctx context.Context, userID string, isAdmin bool) (User, error) {
	if s == nil || s.db == nil {
		return User{}, errors.New("admin database unavailable")
	}
	if strings.TrimSpace(userID) == "" {
		return User{}, ErrInvalidInput
	}

	var account User
	err := s.db.QueryRowContext(ctx, `
		UPDATE users
		SET is_admin = $1
		WHERE id = $2
		RETURNING users.id, users.username, users.email,
			(SELECT plans.name FROM plans WHERE plans.id = users.plan_id), users.is_admin,
			users.monthly_minutes_used, users.monthly_transfer_bytes_used,
			users.created_at, users.last_login_at`, isAdmin, userID).Scan(
		&account.ID, &account.Username, &account.Email, &account.Plan, &account.IsAdmin,
		&account.MonthlyMinutesUsed, &account.MonthlyTransferBytes,
		&account.CreatedAt, &account.LastLoginAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("set user admin permission: %w", err)
	}
	return account, nil
}

func typedCursorValue(value string, field SortField) (any, error) {
	switch field {
	case SortByTransfer, SortByMinutes:
		return strconv.ParseInt(value, 10, 64)
	case SortByCreated:
		return time.Parse(time.RFC3339Nano, value)
	case SortByUsername:
		return value, nil
	default:
		return nil, ErrInvalidInput
	}
}

func decodeCursor(value string) (string, string, error) {
	if value == "" {
		return "", "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", "", ErrInvalidInput
	}
	var token cursor
	if err := json.Unmarshal(decoded, &token); err != nil || token.Value == "" || token.ID == "" {
		return "", "", ErrInvalidInput
	}
	return token.Value, token.ID, nil
}

func encodeCursor(value, id string) string {
	encoded, _ := json.Marshal(cursor{Value: value, ID: id})
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func sortValue(account User, field SortField) string {
	switch field {
	case SortByTransfer:
		return strconv.FormatInt(account.MonthlyTransferBytes, 10)
	case SortByMinutes:
		return strconv.FormatInt(account.MonthlyMinutesUsed, 10)
	case SortByUsername:
		return account.Username
	default:
		return account.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}
