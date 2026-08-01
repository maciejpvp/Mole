package user

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRandomToken(t *testing.T) {
	first, err := randomToken(32)
	if err != nil {
		t.Fatalf("generate first token: %v", err)
	}
	second, err := randomToken(32)
	if err != nil {
		t.Fatalf("generate second token: %v", err)
	}
	if first == second || len(first) != 43 {
		t.Fatalf("unexpected random tokens")
	}
}

func TestListPlans(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, max_active_tunnels")).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "max_active_tunnels", "monthly_minutes", "monthly_transfer_bytes",
		}).AddRow(int64(1), "free", int64(1), int64(60), int64(1073741824)).
			AddRow(int64(3), "unlimited", nil, nil, nil))

	plans, err := NewService(db).ListPlans(context.Background())
	if err != nil {
		t.Fatalf("list plans: %v", err)
	}
	if len(plans) != 2 || plans[0].Name != "free" || plans[1].Name != "unlimited" {
		t.Fatalf("unexpected plans: %+v", plans)
	}
	if plans[1].MaxActiveTunnels != nil || plans[1].MonthlyMinutes != nil || plans[1].MonthlyTransferBytes != nil {
		t.Fatalf("expected unlimited plan quotas to be nil: %+v", plans[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestListPlansWithUnavailableDatabase(t *testing.T) {
	if _, err := (*Service)(nil).ListPlans(context.Background()); err == nil {
		t.Fatal("expected database unavailable error")
	}
}
