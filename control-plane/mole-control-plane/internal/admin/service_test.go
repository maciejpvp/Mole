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
