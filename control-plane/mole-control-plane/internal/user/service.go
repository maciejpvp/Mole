package user

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

const (
	bcryptCost          = 12
	maxFailedAttempts   = 5
	accountLockDuration = 15 * time.Minute
	defaultSessionTTL   = 24 * time.Hour
)

var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrAccountUnavailable = errors.New("account unavailable")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthenticated    = errors.New("unauthenticated")
)

// Service owns user registration, password verification, and session creation.
type Service struct {
	db         *sql.DB
	sessionTTL time.Duration
	now        func() time.Time
}

type RegisterInput struct {
	Username string
	Email    string
	Password string
}

type LoginInput struct {
	Identifier string
	Password   string
}

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Plan     string `json:"plan"`
}

type Authentication struct {
	User      User      `json:"user"`
	Token     string    `json:"access_token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db, sessionTTL: defaultSessionTTL, now: time.Now}
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (Authentication, error) {
	username, email, password, err := normalizeRegistration(input)
	if err != nil {
		return Authentication{}, err
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return Authentication{}, fmt.Errorf("hash password: %w", err)
	}
	userID, err := randomToken(16)
	if err != nil {
		return Authentication{}, fmt.Errorf("generate user ID: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Authentication{}, fmt.Errorf("begin registration: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var planID int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM plans WHERE name = 'free'").Scan(&planID); err != nil {
		return Authentication{}, fmt.Errorf("get free plan: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO users (id, username, email, password_hash, plan_id)
		VALUES ($1, $2, $3, $4, $5)`, userID, username, email, string(passwordHash), planID)
	if err != nil {
		if isUniqueViolation(err) {
			return Authentication{}, ErrAccountUnavailable
		}
		return Authentication{}, fmt.Errorf("create user: %w", err)
	}

	auth, err := s.createSession(ctx, tx, User{ID: userID, Username: username, Email: email, Plan: "free"})
	if err != nil {
		return Authentication{}, err
	}
	if err := tx.Commit(); err != nil {
		return Authentication{}, fmt.Errorf("commit registration: %w", err)
	}
	return auth, nil
}

func (s *Service) Login(ctx context.Context, input LoginInput) (Authentication, error) {
	identifier := strings.ToLower(strings.TrimSpace(input.Identifier))
	if identifier == "" || input.Password == "" || len(input.Password) > 72 {
		return Authentication{}, ErrInvalidCredentials
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Authentication{}, fmt.Errorf("begin login: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var (
		account        User
		passwordHash   string
		failedAttempts int
		lockedUntil    sql.NullTime
	)
	err = tx.QueryRowContext(ctx, `
		SELECT users.id, users.username, users.email, users.password_hash, plans.name,
			users.failed_login_attempts, users.locked_until
		FROM users
		JOIN plans ON plans.id = users.plan_id
		WHERE users.username = $1 OR users.email = $1
		FOR UPDATE`, identifier).Scan(
		&account.ID, &account.Username, &account.Email, &passwordHash, &account.Plan,
		&failedAttempts, &lockedUntil,
	)
	if errors.Is(err, sql.ErrNoRows) {
		// Keep the cost of an unknown account login comparable to a real login.
		_ = bcrypt.CompareHashAndPassword([]byte(dummyPasswordHash), []byte(input.Password))
		return Authentication{}, ErrInvalidCredentials
	}
	if err != nil {
		return Authentication{}, fmt.Errorf("find account: %w", err)
	}

	now := s.now().UTC()
	if lockedUntil.Valid && lockedUntil.Time.After(now) {
		return Authentication{}, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(input.Password)) != nil {
		failedAttempts++
		var newLock sql.NullTime
		if failedAttempts >= maxFailedAttempts {
			newLock = sql.NullTime{Time: now.Add(accountLockDuration), Valid: true}
			failedAttempts = 0
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE users
			SET failed_login_attempts = $1, locked_until = $2
			WHERE id = $3`, failedAttempts, newLock, account.ID); err != nil {
			return Authentication{}, fmt.Errorf("record failed login: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return Authentication{}, fmt.Errorf("commit failed login: %w", err)
		}
		return Authentication{}, ErrInvalidCredentials
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE users
		SET failed_login_attempts = 0, locked_until = NULL, last_login_at = $1
		WHERE id = $2`, now, account.ID); err != nil {
		return Authentication{}, fmt.Errorf("record login: %w", err)
	}
	auth, err := s.createSession(ctx, tx, account)
	if err != nil {
		return Authentication{}, err
	}
	if err := tx.Commit(); err != nil {
		return Authentication{}, fmt.Errorf("commit login: %w", err)
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
		SELECT users.id, users.username, users.email, plans.name
		FROM sessions
		JOIN users ON users.id = sessions.user_id
		JOIN plans ON plans.id = users.plan_id
		WHERE sessions.token_hash = $1 AND sessions.expires_at > CURRENT_TIMESTAMP`, tokenHash[:]).Scan(
		&account.ID, &account.Username, &account.Email, &account.Plan,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUnauthenticated
	}
	if err != nil {
		return User{}, fmt.Errorf("authenticate session: %w", err)
	}
	_, _ = s.db.ExecContext(ctx, "UPDATE sessions SET last_used_at = CURRENT_TIMESTAMP WHERE token_hash = $1", tokenHash[:])
	return account, nil
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

func normalizeRegistration(input RegisterInput) (string, string, string, error) {
	username := strings.ToLower(strings.TrimSpace(input.Username))
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if !validUsername(username) || !validEmail(email) || !validPassword(input.Password) {
		return "", "", "", ErrInvalidInput
	}
	return username, email, input.Password, nil
}

func validUsername(value string) bool {
	if len(value) < 3 || len(value) > 32 {
		return false
	}
	for i, char := range value {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || (i > 0 && (char == '_' || char == '-')) {
			continue
		}
		return false
	}
	return true
}

func validEmail(value string) bool {
	if len(value) > 254 {
		return false
	}
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == value && strings.Count(value, "@") == 1
}

func validPassword(value string) bool {
	return utf8.ValidString(value) && len(value) >= 12 && len(value) <= 72
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

// A valid bcrypt hash used to equalize login timing when no account is found.
const dummyPasswordHash = "$2a$12$LQv3c1yqrv9IXNVzXuD.Tu3W6Lxsa0YQzM8b0e4ujdMZD8ydphbOm"
