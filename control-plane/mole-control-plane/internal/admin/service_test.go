package admin

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

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
			"id", "username", "email", "plan", "is_admin",
			"monthly_minutes_used", "monthly_transfer_bytes_used", "created_at", "last_login_at",
		}).AddRow("user-1", "alice", "alice@example.com", "free", false, 10, 20, createdAt, nil).
			AddRow("user-2", "bob", "bob@example.com", "premium", true, 30, 40, createdAt, nil))

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
			"id", "username", "email", "plan", "is_admin",
			"monthly_minutes_used", "monthly_transfer_bytes_used", "created_at", "last_login_at",
		}).AddRow("user-1", "alice", "alice@example.com", "premium", false, 10, 20, createdAt, nil))
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
			"id", "username", "email", "plan", "is_admin",
			"monthly_minutes_used", "monthly_transfer_bytes_used", "created_at", "last_login_at",
		}).AddRow("user-1", "alice", "alice@example.com", "premium", true, 10, 20, createdAt, nil))

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
