package admin

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

type testDeprovisioner struct {
	ids []string
	err error
}

func (d *testDeprovisioner) Deprovision(_ context.Context, tunnelID string) error {
	d.ids = append(d.ids, tunnelID)
	return d.err
}

func TestListUsersUsesCursorPagination(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT users.id")).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "email", "plan", "is_admin", "is_banned",
			"monthly_minutes_used", "monthly_transfer_bytes_used", "created_at", "last_login_at",
		}).AddRow("user-1", "alice", "alice@example.com", "free", false, false, 10, 20, createdAt, nil).
			AddRow("user-2", "bob", "bob@example.com", "premium", true, false, 30, 40, createdAt, nil))

	page, err := NewService(db).ListUsers(context.Background(), ListUsersInput{
		Limit:     1,
		Sort:      SortByTransfer,
		Direction: Descending,
	})
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(page.Users) != 1 || page.Users[0].ID != "user-1" {
		t.Fatalf("unexpected page: %+v", page)
	}
	if page.NextCursor == "" {
		t.Fatal("expected a next cursor")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(page.NextCursor) {
		t.Fatalf("expected URL-safe cursor, got %q", page.NextCursor)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestListUsersRejectsInvalidQuery(t *testing.T) {
	service := NewService(&sql.DB{})
	for _, input := range []ListUsersInput{
		{Limit: 101},
		{Sort: SortField("password")},
		{Direction: SortDirection("sideways")},
		{Cursor: "not-a-cursor"},
	} {
		if _, err := service.ListUsers(context.Background(), input); err != ErrInvalidInput {
			t.Fatalf("expected ErrInvalidInput for %+v, got %v", input, err)
		}
	}
}

func TestChangeUserPlan(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM plans WHERE id = $1")).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(2)))
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE users")).
		WithArgs(int64(2), "user-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "email", "plan", "is_admin", "is_banned",
			"monthly_minutes_used", "monthly_transfer_bytes_used", "created_at", "last_login_at",
		}).AddRow("user-1", "alice", "alice@example.com", "premium", false, false, 10, 20, createdAt, nil))
	mock.ExpectCommit()

	account, err := NewService(db).ChangeUserPlan(context.Background(), "user-1", 2)
	if err != nil {
		t.Fatalf("change user plan: %v", err)
	}
	if account.ID != "user-1" || account.Plan != "premium" || account.MonthlyMinutesUsed != 10 || account.MonthlyTransferBytes != 20 {
		t.Fatalf("unexpected updated account: %+v", account)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestChangeUserPlanRejectsMissingPlan(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM plans WHERE id = $1")).
		WithArgs(int64(99)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	if _, err := NewService(db).ChangeUserPlan(context.Background(), "user-1", 99); err != ErrPlanNotFound {
		t.Fatalf("expected ErrPlanNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestResetUserLimits(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE users")).
		WithArgs("user-1").WillReturnRows(sqlmock.NewRows([]string{
		"id", "username", "email", "plan", "is_admin", "is_banned",
		"monthly_minutes_used", "monthly_transfer_bytes_used", "created_at", "last_login_at",
	}).AddRow("user-1", "alice", "alice@example.com", "free", false, false, 0, 0, createdAt, nil))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE tunnels")).
		WithArgs("user-1").WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	account, err := NewService(db).ResetUserLimits(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("reset user limits: %v", err)
	}
	if account.ID != "user-1" || account.MonthlyMinutesUsed != 0 || account.MonthlyTransferBytes != 0 {
		t.Fatalf("unexpected reset account: %+v", account)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestResetUserLimitsRejectsMissingUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE users")).
		WithArgs("missing-user").WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	if _, err := NewService(db).ResetUserLimits(context.Background(), "missing-user"); err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestSetUserAdmin(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE users")).
		WithArgs(true, "user-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "username", "email", "plan", "is_admin", "is_banned",
			"monthly_minutes_used", "monthly_transfer_bytes_used", "created_at", "last_login_at",
		}).AddRow("user-1", "alice", "alice@example.com", "premium", true, false, 10, 20, createdAt, nil))

	account, err := NewService(db).SetUserAdmin(context.Background(), "user-1", true)
	if err != nil {
		t.Fatalf("set user admin permission: %v", err)
	}
	if account.ID != "user-1" || !account.IsAdmin {
		t.Fatalf("unexpected updated account: %+v", account)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestSetUserAdminRejectsMissingUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("UPDATE users")).
		WithArgs(false, "user-1").
		WillReturnError(sql.ErrNoRows)

	if _, err := NewService(db).SetUserAdmin(context.Background(), "user-1", false); err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestSetUserBannedRemovesTunnelsBeforeCommit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	defer db.Close()

	deprovisioner := &testDeprovisioner{}
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM users WHERE id = $1 FOR UPDATE")).
		WithArgs("user-1").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("user-1"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM tunnels WHERE user_id = $1 FOR UPDATE")).
		WithArgs("user-1").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("tunnel-1").AddRow("tunnel-2"))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM tunnels WHERE user_id = $1")).WithArgs("user-1").WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE users")).
		WithArgs(true, "user-1").WillReturnRows(sqlmock.NewRows([]string{
		"id", "username", "email", "plan", "is_admin", "is_banned",
		"monthly_minutes_used", "monthly_transfer_bytes_used", "created_at", "last_login_at",
	}).AddRow("user-1", "alice", "alice@example.com", "free", false, true, 0, 0, createdAt, nil))
	mock.ExpectCommit()

	account, err := NewService(db, deprovisioner).SetUserBanned(context.Background(), "user-1", true)
	if err != nil {
		t.Fatalf("set user banned: %v", err)
	}
	if !account.IsBanned || len(deprovisioner.ids) != 2 || deprovisioner.ids[0] != "tunnel-1" || deprovisioner.ids[1] != "tunnel-2" {
		t.Fatalf("unexpected ban result: %+v, deprovisioned=%v", account, deprovisioner.ids)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestSetUserBannedRollsBackWhenTunnelCleanupFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM users WHERE id = $1 FOR UPDATE")).
		WithArgs("user-1").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("user-1"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM tunnels WHERE user_id = $1 FOR UPDATE")).
		WithArgs("user-1").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("tunnel-1"))
	mock.ExpectRollback()

	_, err = NewService(db, &testDeprovisioner{err: errors.New("relay unavailable")}).SetUserBanned(context.Background(), "user-1", true)
	if !errors.Is(err, ErrTunnelCleanup) {
		t.Fatalf("expected tunnel cleanup error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}
