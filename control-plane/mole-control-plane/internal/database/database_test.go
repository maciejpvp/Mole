package database

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func mustStartPostgresContainer() (func(context.Context, ...testcontainers.TerminateOption) error, error) {
	var (
		dbName = "database"
		dbPwd  = "password"
		dbUser = "user"
	)

	dbContainer, err := postgres.Run(
		context.Background(),
		"postgres:latest",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPwd),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)
	if err != nil {
		return nil, err
	}

	database = dbName
	password = dbPwd
	username = dbUser

	dbHost, err := dbContainer.Host(context.Background())
	if err != nil {
		return dbContainer.Terminate, err
	}

	dbPort, err := dbContainer.MappedPort(context.Background(), "5432/tcp")
	if err != nil {
		return dbContainer.Terminate, err
	}

	host = dbHost
	port = dbPort.Port()

	return dbContainer.Terminate, err
}

func TestMain(m *testing.M) {
	teardown, err := mustStartPostgresContainer()
	if err != nil {
		log.Fatalf("could not start postgres container: %v", err)
	}

	m.Run()

	if teardown != nil && teardown(context.Background()) != nil {
		log.Fatalf("could not teardown postgres container: %v", err)
	}
}

func TestNew(t *testing.T) {
	srv := New()
	if srv == nil {
		t.Fatal("New() returned nil")
	}
}

func TestHealth(t *testing.T) {
	srv := New()

	stats := srv.Health()

	if stats["status"] != "up" {
		t.Fatalf("expected status to be up, got %s", stats["status"])
	}

	if _, ok := stats["error"]; ok {
		t.Fatalf("expected error not to be present")
	}

	if stats["message"] != "It's healthy" {
		t.Fatalf("expected message to be 'It's healthy', got %s", stats["message"])
	}
}

func TestPlansMigration(t *testing.T) {
	srv := New().(*service)

	rows, err := srv.db.Query(`
		SELECT name, max_active_tunnels, monthly_minutes, monthly_transfer_bytes
		FROM plans
		ORDER BY name`)
	if err != nil {
		t.Fatalf("query plans: %v", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var (
			name                 string
			maxActiveTunnels     *int
			monthlyMinutes       *int64
			monthlyTransferBytes *int64
		)
		if err := rows.Scan(&name, &maxActiveTunnels, &monthlyMinutes, &monthlyTransferBytes); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		switch name {
		case "free":
			if maxActiveTunnels == nil || *maxActiveTunnels != 1 || monthlyMinutes == nil || *monthlyMinutes != 60 || monthlyTransferBytes == nil || *monthlyTransferBytes != 1073741824 {
				t.Fatalf("unexpected free plan limits")
			}
		case "premium":
			if maxActiveTunnels == nil || *maxActiveTunnels != 3 || monthlyMinutes == nil || *monthlyMinutes != 260 || monthlyTransferBytes == nil || *monthlyTransferBytes != 10737418240 {
				t.Fatalf("unexpected premium plan limits")
			}
		case "unlimited":
			if maxActiveTunnels != nil || monthlyMinutes != nil || monthlyTransferBytes != nil {
				t.Fatalf("expected unlimited plan limits to be unset")
			}
		default:
			t.Fatalf("unexpected plan %q", name)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate plans: %v", err)
	}

	expected := []string{"free", "premium", "unlimited"}
	if len(names) != len(expected) {
		t.Fatalf("expected plans %v, got %v", expected, names)
	}
	for i, name := range expected {
		if names[i] != name {
			t.Fatalf("expected plans %v, got %v", expected, names)
		}
	}
}

func TestTunnelsMigration(t *testing.T) {
	srv := New().(*service)
	createTestUser(t, srv, "test-user")

	var tunnelID string
	err := srv.db.QueryRow(`
		INSERT INTO tunnels (id, user_id, outbound_port, inbound_ip, inbound_port, server_address, connection_token_hash)
		VALUES ('test-tunnel', 'test-user', 8080, '127.0.0.1', 3000, 'relay.example.test:9000', decode('00', 'hex'))
		RETURNING id`).Scan(&tunnelID)
	if err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	t.Cleanup(func() {
		if _, err := srv.db.Exec("DELETE FROM tunnels WHERE id = $1", tunnelID); err != nil {
			t.Errorf("delete test tunnel: %v", err)
		}
	})

	var (
		userID                     string
		outboundPort               int
		inboundIP                  string
		inboundPort                int
		status                     string
		currentPeriodMinutes       int64
		currentPeriodTransferBytes int64
	)
	if err := srv.db.QueryRow(`
		SELECT user_id, outbound_port, host(inbound_ip), inbound_port, status,
			current_period_minutes, current_period_transfer_bytes
		FROM tunnels
		WHERE id = $1`, tunnelID).Scan(
		&userID,
		&outboundPort,
		&inboundIP,
		&inboundPort,
		&status,
		&currentPeriodMinutes,
		&currentPeriodTransferBytes,
	); err != nil {
		t.Fatalf("query tunnel: %v", err)
	}

	if userID != "test-user" || outboundPort != 8080 || inboundIP != "127.0.0.1" || inboundPort != 3000 || status != "inactive" || currentPeriodMinutes != 0 || currentPeriodTransferBytes != 0 {
		t.Fatalf("unexpected tunnel values: %q, %d, %q, %d, %q, %d, %d", userID, outboundPort, inboundIP, inboundPort, status, currentPeriodMinutes, currentPeriodTransferBytes)
	}
}

func TestUsersMigration(t *testing.T) {
	srv := New().(*service)
	createTestUser(t, srv, "another-test-user")

	var (
		planName                 string
		monthlyMinutesUsed       int64
		monthlyTransferBytesUsed int64
		usageLimitReachedAt      *time.Time
	)
	if err := srv.db.QueryRow(`
		SELECT plans.name, users.monthly_minutes_used, users.monthly_transfer_bytes_used,
			users.usage_limit_reached_at
		FROM users
		JOIN plans ON plans.id = users.plan_id
		WHERE users.id = 'another-test-user'`).Scan(
		&planName,
		&monthlyMinutesUsed,
		&monthlyTransferBytesUsed,
		&usageLimitReachedAt,
	); err != nil {
		t.Fatalf("query user: %v", err)
	}
	if planName != "free" || monthlyMinutesUsed != 0 || monthlyTransferBytesUsed != 0 || usageLimitReachedAt != nil {
		t.Fatalf("unexpected user usage values")
	}
}

func createTestUser(t *testing.T, srv *service, userID string) {
	t.Helper()

	var planID int64
	if err := srv.db.QueryRow("SELECT id FROM plans WHERE name = 'free'").Scan(&planID); err != nil {
		t.Fatalf("get free plan: %v", err)
	}
	if _, err := srv.db.Exec(`
		INSERT INTO users (id, username, email, password_hash, plan_id)
		VALUES ($1, $2, $3, '$2a$12$LQv3c1yqrv9IXNVzXuD.Tu3W6Lxsa0YQzM8b0e4ujdMZD8ydphbOm', $4)`,
		userID, userID, userID+"@example.com", planID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		if _, err := srv.db.Exec("DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete test user: %v", err)
		}
	})
}

func TestClose(t *testing.T) {
	srv := New()

	if srv.Close() != nil {
		t.Fatalf("expected Close() to return nil")
	}
}
