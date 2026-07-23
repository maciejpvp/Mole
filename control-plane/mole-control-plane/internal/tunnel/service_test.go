package tunnel

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestValidateInput(t *testing.T) {
	protocol, ip, port, err := validateInput(CreateInput{Protocol: " TCP ", InternalAddress: "127.0.0.1:25565"})
	if err != nil {
		t.Fatalf("validate TCP input: %v", err)
	}
	if protocol != "tcp" || ip.String() != "127.0.0.1" || port != 25565 {
		t.Fatalf("unexpected parsed input: %q, %s, %d", protocol, ip, port)
	}
}

func TestValidateInputRejectsInvalidEndpoint(t *testing.T) {
	tests := []CreateInput{
		{Protocol: "icmp", InternalAddress: "127.0.0.1:25565"},
		{Protocol: "tcp", InternalAddress: "localhost:25565"},
		{Protocol: "udp", InternalAddress: "127.0.0.1:0"},
		{Protocol: "udp", InternalAddress: "127.0.0.1"},
	}
	for _, input := range tests {
		if _, _, _, err := validateInput(input); err != ErrInvalidInput {
			t.Fatalf("expected invalid input for %+v, got %v", input, err)
		}
	}
}

type mockProvisioner struct {
	outboundPort int
	publicHost   string
	controlPort  int
	err          error
}

func (m *mockProvisioner) Provision(ctx context.Context, req ProvisionRequest) (ProvisionResponse, error) {
	if m.err != nil {
		return ProvisionResponse{}, m.err
	}
	return ProvisionResponse{
		OutboundPort: m.outboundPort,
		PublicHost:   m.publicHost,
		ControlPort:  m.controlPort,
	}, nil
}

func (m *mockProvisioner) Deprovision(ctx context.Context, tunnelID string) error {
	return nil
}

func TestCreateTunnelEnforcesMaxActiveTunnels(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("unexpected error creating sqlmock: %v", err)
	}
	defer db.Close()

	svc := NewService(db, &mockProvisioner{outboundPort: 10001, publicHost: "relay.test", controlPort: 9001})

	// Case 1: User is on free plan with max_active_tunnels = 1, and has 1 inactive tunnel already.
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT plans.max_active_tunnels`).
		WithArgs("test-user-id").
		WillReturnRows(sqlmock.NewRows([]string{
			"max_active_tunnels", "monthly_minutes", "monthly_transfer_bytes",
			"monthly_minutes_used", "monthly_transfer_bytes_used", "usage_period_started_at",
		}).AddRow(1, nil, nil, 0, 0, time.Now()))

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM tunnels WHERE user_id = \$1 AND status IN \('inactive', 'active'\)`).
		WithArgs("test-user-id").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectRollback()

	_, err = svc.Create(context.Background(), "test-user-id", CreateInput{
		Protocol:        "tcp",
		InternalAddress: "127.0.0.1:8080",
	})
	if err != ErrLimitReached {
		t.Fatalf("expected ErrLimitReached when user has 1 inactive tunnel on max_active_tunnels=1 plan, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestSetConnectionStatusEnforcesLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("unexpected error creating sqlmock: %v", err)
	}
	defer db.Close()

	svc := NewService(db, &mockProvisioner{})

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT user_id, status FROM tunnels WHERE id = \$1 AND status IN \('inactive', 'active'\)`).
		WithArgs("tunnel-1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "status"}).AddRow("test-user-id", "inactive"))

	mock.ExpectQuery(`SELECT plans.max_active_tunnels`).
		WithArgs("test-user-id").
		WillReturnRows(sqlmock.NewRows([]string{
			"max_active_tunnels", "monthly_minutes", "monthly_transfer_bytes",
			"monthly_minutes_used", "monthly_transfer_bytes_used", "usage_period_started_at",
		}).AddRow(1, nil, nil, 0, 0, time.Now()))

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM tunnels WHERE user_id = \$1 AND status = 'active'`).
		WithArgs("test-user-id").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectRollback()

	err = svc.SetConnectionStatus(context.Background(), "tunnel-1", "active")
	if err != ErrLimitReached {
		t.Fatalf("expected ErrLimitReached when user already has max active tunnels, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}
