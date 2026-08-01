package user

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	defaultSessionTTL = 24 * time.Hour
	loginCodeTTL      = 60 * time.Second
)

var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrAccountUnavailable = errors.New("account unavailable")
	ErrUnauthenticated    = errors.New("unauthenticated")
)

// Service owns Google identity provisioning and session creation.
type Service struct {
	db         *sql.DB
	sessionTTL time.Duration
	now        func() time.Time
}

type GoogleIdentity struct {
	Subject string
	Email   string
}

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Plan     string `json:"plan"`
	IsAdmin  bool   `json:"-"`
	IsBanned bool   `json:"-"`
}

// Profile is the authenticated user's account snapshot. It intentionally
// excludes credentials, session tokens, and tunnel connection tokens.
type Profile struct {
	User
	IsAdmin     bool       `json:"is_admin,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
	Limits      Limits     `json:"limits"`
	Usage       Usage      `json:"usage"`
	Tunnels     []Tunnel   `json:"tunnels"`
}

type Limits struct {
	MaxActiveTunnels     *int64 `json:"max_active_tunnels"`
	MonthlyMinutes       *int64 `json:"monthly_minutes"`
	MonthlyTransferBytes *int64 `json:"monthly_transfer_bytes"`
}

type Plan struct {
	ID                   int64  `json:"id"`
	Name                 string `json:"name"`
	MaxActiveTunnels     *int64 `json:"max_active_tunnels"`
	MonthlyMinutes       *int64 `json:"monthly_minutes"`
	MonthlyTransferBytes *int64 `json:"monthly_transfer_bytes"`
}

type Usage struct {
	PeriodStartedAt      time.Time  `json:"period_started_at"`
	MonthlyMinutesUsed   int64      `json:"monthly_minutes_used"`
	MonthlyTransferBytes int64      `json:"monthly_transfer_bytes_used"`
	LimitReachedAt       *time.Time `json:"limit_reached_at"`
}

type Tunnel struct {
	ID                         string     `json:"id"`
	Protocol                   string     `json:"proto"`
	InternalAddress            string     `json:"internal_address"`
	OutboundPort               int        `json:"outbound_port"`
	ServerAddress              string     `json:"server_address"`
	Status                     string     `json:"status"`
	StartedAt                  *time.Time `json:"started_at"`
	StoppedAt                  *time.Time `json:"stopped_at"`
	CurrentPeriodMinutes       int64      `json:"current_period_minutes"`
	CurrentPeriodTransferBytes int64      `json:"current_period_transfer_bytes"`
	CreatedAt                  time.Time  `json:"created_at"`
}

type Authentication struct {
	User      User      `json:"user"`
	Token     string    `json:"access_token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db, sessionTTL: defaultSessionTTL, now: time.Now}
}

func (s *Service) ListPlans(ctx context.Context) ([]Plan, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("user database unavailable")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, max_active_tunnels, monthly_minutes, monthly_transfer_bytes
		FROM plans
		ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()

	plans := make([]Plan, 0)
	for rows.Next() {
		var plan Plan
		if err := rows.Scan(&plan.ID, &plan.Name, &plan.MaxActiveTunnels, &plan.MonthlyMinutes, &plan.MonthlyTransferBytes); err != nil {
			return nil, fmt.Errorf("scan plan: %w", err)
		}
		plans = append(plans, plan)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plans: %w", err)
	}
	return plans, nil
}

func (s *Service) LoginWithGoogle(ctx context.Context, identity GoogleIdentity) (string, error) {
	subject := strings.TrimSpace(identity.Subject)
	email := strings.ToLower(strings.TrimSpace(identity.Email))
	if subject == "" || !validEmail(email) {
		return "", ErrInvalidInput
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin Google login: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var account User
	err = tx.QueryRowContext(ctx, `
		SELECT users.id, users.username, users.email, plans.name, users.is_banned
		FROM users JOIN plans ON plans.id = users.plan_id
		WHERE users.google_subject = $1 FOR UPDATE`, subject).Scan(
		&account.ID, &account.Username, &account.Email, &account.Plan, &account.IsBanned,
	)
	now := s.now().UTC()
	if errors.Is(err, sql.ErrNoRows) {
		userID, idErr := randomToken(16)
		if idErr != nil {
			return "", fmt.Errorf("generate user ID: %w", idErr)
		}
		username, nameErr := s.nextUsername(ctx, tx, email, subject)
		if nameErr != nil {
			return "", nameErr
		}
		var planID int64
		if err := tx.QueryRowContext(ctx, "SELECT id FROM plans WHERE name = 'free'").Scan(&planID); err != nil {
			return "", fmt.Errorf("get free plan: %w", err)
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO users (id, username, email, google_subject, plan_id, last_login_at)
			VALUES ($1, $2, $3, $4, $5, $6)`, userID, username, email, subject, planID, now)
		if err != nil {
			if isUniqueViolation(err) {
				return "", ErrAccountUnavailable
			}
			return "", fmt.Errorf("create Google user: %w", err)
		}
		account = User{ID: userID, Username: username, Email: email, Plan: "free"}
	} else if err != nil {
		return "", fmt.Errorf("find Google account: %w", err)
	} else {
		if account.IsBanned {
			return "", ErrUnauthenticated
		}
		if _, err := tx.ExecContext(ctx, "UPDATE users SET email = $1, last_login_at = $2 WHERE id = $3", email, now, account.ID); err != nil {
			return "", fmt.Errorf("record Google login: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit Google login: %w", err)
	}
	return account.ID, nil
}

func (s *Service) nextUsername(ctx context.Context, tx *sql.Tx, email, subject string) (string, error) {
	local := strings.ToLower(strings.SplitN(email, "@", 2)[0])
	var builder strings.Builder
	for _, char := range local {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_' || char == '-' {
			builder.WriteRune(char)
		}
	}
	base := builder.String()
	base = strings.TrimLeft(base, "_-")
	if len(base) < 3 {
		base = "user"
	}
	if len(base) > 32 {
		base = base[:32]
	}
	username := base
	var taken bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE username = $1)", username).Scan(&taken); err != nil {
		return "", fmt.Errorf("check username: %w", err)
	}
	if !taken {
		return username, nil
	}
	hash := sha256.Sum256([]byte(subject))
	suffix := fmt.Sprintf("-%x", hash[:3])
	prefixLength := 32 - len(suffix)
	if len(base) > prefixLength {
		base = base[:prefixLength]
	}
	username = base + suffix
	for attempt := 1; ; attempt++ {
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE username = $1)", username).Scan(&taken); err != nil {
			return "", fmt.Errorf("check username suffix: %w", err)
		}
		if !taken {
			return username, nil
		}
		candidate := fmt.Sprintf("%s-%d", base, attempt)
		if len(candidate) > 32 {
			candidate = candidate[:32]
		}
		username = candidate
	}
}

func (s *Service) CreateLoginCode(ctx context.Context, userID string) (string, error) {
	code, err := randomToken(32)
	if err != nil {
		return "", fmt.Errorf("generate login code: %w", err)
	}
	hash := sha256.Sum256([]byte(code))
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO auth_login_codes (code_hash, user_id, expires_at)
		VALUES ($1, $2, $3)`, hash[:], userID, s.now().UTC().Add(loginCodeTTL)); err != nil {
		return "", fmt.Errorf("store login code: %w", err)
	}
	return code, nil
}

func (s *Service) ExchangeLoginCode(ctx context.Context, code string) (Authentication, error) {
	if strings.TrimSpace(code) == "" {
		return Authentication{}, ErrUnauthenticated
	}
	hash := sha256.Sum256([]byte(code))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Authentication{}, fmt.Errorf("begin login code exchange: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	var account User
	var expiresAt time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT users.id, users.username, users.email, plans.name, users.is_admin, users.is_banned, auth_login_codes.expires_at
		FROM auth_login_codes
		JOIN users ON users.id = auth_login_codes.user_id
		JOIN plans ON plans.id = users.plan_id
		WHERE auth_login_codes.code_hash = $1 AND auth_login_codes.used_at IS NULL
		FOR UPDATE`, hash[:]).Scan(&account.ID, &account.Username, &account.Email, &account.Plan, &account.IsAdmin, &account.IsBanned, &expiresAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Authentication{}, fmt.Errorf("find login code: %w", err)
	}
	if errors.Is(err, sql.ErrNoRows) || expiresAt.Before(s.now().UTC()) || account.IsBanned {
		return Authentication{}, ErrUnauthenticated
	}
	if _, err := tx.ExecContext(ctx, "UPDATE auth_login_codes SET used_at = $1 WHERE code_hash = $2", s.now().UTC(), hash[:]); err != nil {
		return Authentication{}, fmt.Errorf("consume login code: %w", err)
	}
	auth, err := s.createSession(ctx, tx, account)
	if err != nil {
		return Authentication{}, err
	}
	if err := tx.Commit(); err != nil {
		return Authentication{}, fmt.Errorf("commit login code exchange: %w", err)
	}
	return auth, nil
}

// Authenticate resolves an unexpired opaque session token to its user.
func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	if strings.TrimSpace(token) == "" {
		return User{}, ErrUnauthenticated
	}

	tokenHash := sha256.Sum256([]byte(token))
	var account User
	err := s.db.QueryRowContext(ctx, `
		SELECT users.id, users.username, users.email, plans.name, users.is_admin, users.is_banned
		FROM sessions
		JOIN users ON users.id = sessions.user_id
		JOIN plans ON plans.id = users.plan_id
		WHERE sessions.token_hash = $1 AND sessions.expires_at > CURRENT_TIMESTAMP`, tokenHash[:]).Scan(
		&account.ID, &account.Username, &account.Email, &account.Plan, &account.IsAdmin, &account.IsBanned,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUnauthenticated
	}
	if err != nil {
		return User{}, fmt.Errorf("authenticate session: %w", err)
	}
	if account.IsBanned {
		return User{}, ErrUnauthenticated
	}
	_, _ = s.db.ExecContext(ctx, "UPDATE sessions SET last_used_at = CURRENT_TIMESTAMP WHERE token_hash = $1", tokenHash[:])
	return account, nil
}

// Profile returns all non-sensitive account, quota, usage, and tunnel data
// for an authenticated user.
func (s *Service) Profile(ctx context.Context, userID string) (Profile, error) {
	var (
		profile Profile
		limits  Limits
		usage   Usage
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT users.id, users.username, users.email, plans.name,
			users.is_admin, users.created_at, users.last_login_at,
			plans.max_active_tunnels, plans.monthly_minutes, plans.monthly_transfer_bytes,
			users.usage_period_started_at, users.monthly_minutes_used,
			users.monthly_transfer_bytes_used, users.usage_limit_reached_at
		FROM users
		JOIN plans ON plans.id = users.plan_id
		WHERE users.id = $1`, userID).Scan(
		&profile.ID, &profile.Username, &profile.Email, &profile.Plan,
		&profile.IsAdmin, &profile.CreatedAt, &profile.LastLoginAt,
		&limits.MaxActiveTunnels, &limits.MonthlyMinutes, &limits.MonthlyTransferBytes,
		&usage.PeriodStartedAt, &usage.MonthlyMinutesUsed,
		&usage.MonthlyTransferBytes, &usage.LimitReachedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrUnauthenticated
	}
	if err != nil {
		return Profile{}, fmt.Errorf("get user profile: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, proto, host(inbound_ip), inbound_port, outbound_port, server_address,
			status, started_at, stopped_at, current_period_minutes,
			current_period_transfer_bytes, created_at
		FROM tunnels
		WHERE user_id = $1
		ORDER BY created_at DESC`, userID)
	if err != nil {
		return Profile{}, fmt.Errorf("list user tunnels: %w", err)
	}
	defer rows.Close()

	profile.Tunnels = make([]Tunnel, 0)
	for rows.Next() {
		var (
			entry        Tunnel
			protocol     int16
			internalIP   string
			internalPort int
		)
		if err := rows.Scan(
			&entry.ID, &protocol, &internalIP, &internalPort, &entry.OutboundPort, &entry.ServerAddress,
			&entry.Status, &entry.StartedAt, &entry.StoppedAt, &entry.CurrentPeriodMinutes,
			&entry.CurrentPeriodTransferBytes, &entry.CreatedAt,
		); err != nil {
			return Profile{}, fmt.Errorf("read user tunnel: %w", err)
		}
		switch protocol {
		case 6:
			entry.Protocol = "tcp"
		case 17:
			entry.Protocol = "udp"
		default:
			return Profile{}, fmt.Errorf("read user tunnel: unsupported protocol")
		}
		entry.InternalAddress = net.JoinHostPort(internalIP, strconv.Itoa(internalPort))
		profile.Tunnels = append(profile.Tunnels, entry)
	}
	if err := rows.Err(); err != nil {
		return Profile{}, fmt.Errorf("list user tunnels: %w", err)
	}

	profile.Limits = limits
	profile.Usage = usage
	return profile, nil
}

func (s *Service) createSession(ctx context.Context, tx *sql.Tx, account User) (Authentication, error) {
	token, err := randomToken(32)
	if err != nil {
		return Authentication{}, fmt.Errorf("generate session token: %w", err)
	}
	tokenHash := sha256.Sum256([]byte(token))
	expiresAt := s.now().UTC().Add(s.sessionTTL)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)`, account.ID, tokenHash[:], expiresAt); err != nil {
		return Authentication{}, fmt.Errorf("create session: %w", err)
	}
	return Authentication{User: account, Token: token, ExpiresAt: expiresAt}, nil
}

func validEmail(value string) bool {
	if len(value) > 254 {
		return false
	}
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == value && strings.Count(value, "@") == 1
}

func randomToken(byteLength int) (string, error) {
	bytes := make([]byte, byteLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
