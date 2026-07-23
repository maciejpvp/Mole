package tunnel

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidInput = errors.New("invalid tunnel input")
	ErrLimitReached = errors.New("plan limit reached")
	ErrUnavailable  = errors.New("tunnel server unavailable")
	ErrNotFound     = errors.New("tunnel not found")
)

// Provisioner allocates and releases public listeners on a tunnel server.
type Provisioner interface {
	Provision(context.Context, ProvisionRequest) (ProvisionResponse, error)
	Deprovision(context.Context, string) error
}

type Service struct {
	db          *sql.DB
	provisioner Provisioner
	now         func() time.Time
}

type CreateInput struct {
	Protocol        string
	InternalAddress string
}

type Tunnel struct {
	ID              string `json:"id"`
	Protocol        string `json:"proto"`
	InternalAddress string `json:"internal_address"`
	OutboundPort    int    `json:"outbound_port"`
	Endpoint        string `json:"endpoint"`
	ServerAddress   string `json:"server_address"`
	Token           string `json:"token"`
}

// ConnectionConfig contains the configuration the client needs after proving
// possession of the tunnel token.
type ConnectionConfig struct {
	Protocol        string `json:"proto"`
	InternalAddress string `json:"internal_address"`
	ServerAddress   string `json:"server_address"`
}

type ProvisionRequest struct {
	TunnelID                  string `json:"tunnel_id"`
	UserID                    string `json:"user_id"`
	Protocol                  string `json:"protocol"`
	Token                     string `json:"token"`
	MonthlyMinutesLimit       *int64 `json:"monthly_minutes_limit"`
	MonthlyTransferBytesLimit *int64 `json:"monthly_transfer_bytes_limit"`
	MonthlyMinutesUsed        int64  `json:"monthly_minutes_used"`
	MonthlyTransferBytesUsed  int64  `json:"monthly_transfer_bytes_used"`
}

type ProvisionResponse struct {
	OutboundPort int    `json:"outbound_port"`
	PublicHost   string `json:"public_host"`
	ControlPort  int    `json:"control_port"`
}

type UsageUpdate struct {
	TunnelID      string `json:"tunnel_id"`
	ActiveMinutes int64  `json:"active_minutes"`
	TransferBytes int64  `json:"transfer_bytes"`
}

type UsageSyncResponse struct {
	StopTunnelIDs []string `json:"stop_tunnel_ids"`
}

func NewService(db *sql.DB, provisioner Provisioner) *Service {
	return &Service{db: db, provisioner: provisioner, now: time.Now}
}

func (s *Service) Create(ctx context.Context, userID string, input CreateInput) (Tunnel, error) {
	protocol, internalIP, internalPort, err := validateInput(input)
	if err != nil {
		return Tunnel{}, err
	}
	if s.provisioner == nil {
		return Tunnel{}, ErrUnavailable
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Tunnel{}, fmt.Errorf("begin tunnel creation: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	now := s.now().UTC()
	limits, err := lockAndRefreshUserUsage(ctx, tx, userID, now)
	if err != nil {
		return Tunnel{}, err
	}
	if usageAtLimit(limits) {
		return Tunnel{}, ErrLimitReached
	}

	var activeCount int64
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM tunnels WHERE user_id = $1 AND status IN ('inactive', 'active')", userID).Scan(&activeCount); err != nil {
		return Tunnel{}, fmt.Errorf("count active tunnels: %w", err)
	}
	if limits.maxActiveTunnels.Valid && activeCount >= limits.maxActiveTunnels.Int64 {
		return Tunnel{}, ErrLimitReached
	}

	tunnelID, err := secureToken(16)
	if err != nil {
		return Tunnel{}, fmt.Errorf("generate tunnel ID: %w", err)
	}
	connectionToken, err := secureToken(32)
	if err != nil {
		return Tunnel{}, fmt.Errorf("generate tunnel token: %w", err)
	}
	provisioned, err := s.provisioner.Provision(ctx, ProvisionRequest{
		TunnelID:                  tunnelID,
		UserID:                    userID,
		Protocol:                  protocol,
		Token:                     connectionToken,
		MonthlyMinutesLimit:       nullInt64Pointer(limits.monthlyMinutesLimit),
		MonthlyTransferBytesLimit: nullInt64Pointer(limits.monthlyTransferLimit),
		MonthlyMinutesUsed:        limits.monthlyMinutesUsed,
		MonthlyTransferBytesUsed:  limits.monthlyTransferUsed,
	})
	if err != nil {
		return Tunnel{}, fmt.Errorf("provision public listener: %w", err)
	}
	if provisioned.OutboundPort < 1 || provisioned.OutboundPort > 65535 || provisioned.PublicHost == "" || provisioned.ControlPort < 1 || provisioned.ControlPort > 65535 {
		return Tunnel{}, fmt.Errorf("provision public listener: invalid response")
	}
	cleanupProvision := true
	defer func() {
		if cleanupProvision {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = s.provisioner.Deprovision(cleanupCtx, tunnelID)
		}
	}()

	tokenHash := sha256.Sum256([]byte(connectionToken))
	protoNumber := int16(6)
	if protocol == "udp" {
		protoNumber = 17
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO tunnels (
			id, user_id, proto, outbound_port, inbound_ip, inbound_port,
			server_address, connection_token_hash, status, started_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'inactive', NULL)`,
		tunnelID, userID, protoNumber, provisioned.OutboundPort, internalIP.String(), internalPort,
		net.JoinHostPort(provisioned.PublicHost, strconv.Itoa(provisioned.ControlPort)), tokenHash[:],
	)
	if err != nil {
		return Tunnel{}, fmt.Errorf("store tunnel: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Tunnel{}, fmt.Errorf("commit tunnel creation: %w", err)
	}
	cleanupProvision = false

	return Tunnel{
		ID:              tunnelID,
		Protocol:        protocol,
		InternalAddress: net.JoinHostPort(internalIP.String(), strconv.Itoa(internalPort)),
		OutboundPort:    provisioned.OutboundPort,
		Endpoint:        net.JoinHostPort(provisioned.PublicHost, strconv.Itoa(provisioned.OutboundPort)),
		ServerAddress:   net.JoinHostPort(provisioned.PublicHost, strconv.Itoa(provisioned.ControlPort)),
		Token:           connectionToken,
	}, nil
}

// Delete removes a user's tunnel from the control plane and releases its
// public listener on the relay.
func (s *Service) Delete(ctx context.Context, userID, tunnelID string) error {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(tunnelID) == "" {
		return ErrInvalidInput
	}
	if s.provisioner == nil {
		return ErrUnavailable
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin tunnel deletion: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var storedID string
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM tunnels
		WHERE id = $1 AND user_id = $2
		FOR UPDATE`, tunnelID, userID).Scan(&storedID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("find tunnel for deletion: %w", err)
	}

	if err := s.provisioner.Deprovision(ctx, storedID); err != nil {
		return fmt.Errorf("deprovision tunnel: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM tunnels WHERE id = $1", storedID); err != nil {
		return fmt.Errorf("delete tunnel: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tunnel deletion: %w", err)
	}
	return nil
}

// ConnectionConfigForToken resolves a provisioned tunnel without requiring a
// user session. The token is generated and persisted by the control plane; the
// relay only receives it to authenticate the client connection.
func (s *Service) ConnectionConfigForToken(ctx context.Context, token string) (ConnectionConfig, error) {
	if strings.TrimSpace(token) == "" {
		return ConnectionConfig{}, ErrNotFound
	}
	tokenHash := sha256.Sum256([]byte(token))
	var (
		config       ConnectionConfig
		internalIP   string
		internalPort int
		proto        int16
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT host(inbound_ip), inbound_port, proto, server_address
		FROM tunnels
		WHERE connection_token_hash = $1 AND status IN ('inactive', 'active')`, tokenHash[:]).Scan(
		&internalIP, &internalPort, &proto, &config.ServerAddress,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ConnectionConfig{}, ErrNotFound
	}
	if err != nil {
		return ConnectionConfig{}, fmt.Errorf("resolve tunnel configuration: %w", err)
	}
	if proto == 6 {
		config.Protocol = "tcp"
	} else if proto == 17 {
		config.Protocol = "udp"
	} else {
		return ConnectionConfig{}, fmt.Errorf("resolve tunnel configuration: unsupported protocol")
	}
	config.InternalAddress = net.JoinHostPort(internalIP, strconv.Itoa(internalPort))
	return config, nil
}

// SetConnectionStatus records the relay's authoritative client connection
// state. A tunnel starts inactive and becomes active only after the relay has
// authenticated a client connection.
func (s *Service) SetConnectionStatus(ctx context.Context, tunnelID, status string) error {
	if strings.TrimSpace(tunnelID) == "" || (status != "active" && status != "inactive") {
		return ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin set connection status: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var (
		userID        string
		currentStatus string
	)
	err = tx.QueryRowContext(ctx, `
		SELECT user_id, status FROM tunnels
		WHERE id = $1 AND status IN ('inactive', 'active')
		FOR UPDATE`, tunnelID).Scan(&userID, &currentStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("find tunnel for connection status: %w", err)
	}

	if status == "active" && currentStatus == "inactive" {
		limits, err := lockAndRefreshUserUsage(ctx, tx, userID, s.now().UTC())
		if err != nil {
			return err
		}
		var activeCount int64
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM tunnels WHERE user_id = $1 AND status = 'active'", userID).Scan(&activeCount); err != nil {
			return fmt.Errorf("count active tunnels: %w", err)
		}
		if limits.maxActiveTunnels.Valid && activeCount >= limits.maxActiveTunnels.Int64 {
			return ErrLimitReached
		}
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE tunnels
		SET status = $1,
			started_at = CASE WHEN $1 = 'active' THEN CURRENT_TIMESTAMP ELSE NULL END
		WHERE id = $2`, status, tunnelID)
	if err != nil {
		return fmt.Errorf("set tunnel connection status: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tunnel connection status: %w", err)
	}
	return nil
}

// ApplyUsage persists deltas reported by a tunnel server and returns every
// active tunnel that must be stopped because the user's plan was exceeded.
func (s *Service) ApplyUsage(ctx context.Context, updates []UsageUpdate) (UsageSyncResponse, error) {
	stops := make(map[string]struct{})
	for _, update := range updates {
		if update.TunnelID == "" || update.ActiveMinutes < 0 || update.TransferBytes < 0 {
			return UsageSyncResponse{}, ErrInvalidInput
		}
		if update.ActiveMinutes == 0 && update.TransferBytes == 0 {
			continue
		}
		if err := s.applyUsageUpdate(ctx, update, stops); err != nil {
			return UsageSyncResponse{}, err
		}
	}
	response := UsageSyncResponse{StopTunnelIDs: make([]string, 0, len(stops))}
	for tunnelID := range stops {
		response.StopTunnelIDs = append(response.StopTunnelIDs, tunnelID)
	}
	return response, nil
}

type userLimits struct {
	monthlyMinutesLimit  sql.NullInt64
	monthlyTransferLimit sql.NullInt64
	maxActiveTunnels     sql.NullInt64
	monthlyMinutesUsed   int64
	monthlyTransferUsed  int64
}

func (s *Service) applyUsageUpdate(ctx context.Context, update UsageUpdate, stops map[string]struct{}) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin usage update: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var userID string
	err = tx.QueryRowContext(ctx, "SELECT user_id FROM tunnels WHERE id = $1 FOR UPDATE", update.TunnelID).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // The tunnel was removed while the server still had a pending delta.
	}
	if err != nil {
		return fmt.Errorf("find tunnel for usage: %w", err)
	}

	limits, err := lockAndRefreshUserUsage(ctx, tx, userID, s.now().UTC())
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE tunnels
		SET current_period_minutes = current_period_minutes + $1,
			current_period_transfer_bytes = current_period_transfer_bytes + $2
		WHERE id = $3`, update.ActiveMinutes, update.TransferBytes, update.TunnelID); err != nil {
		return fmt.Errorf("update tunnel usage: %w", err)
	}
	limits.monthlyMinutesUsed += update.ActiveMinutes
	limits.monthlyTransferUsed += update.TransferBytes
	if _, err := tx.ExecContext(ctx, `
		UPDATE users
		SET monthly_minutes_used = $1, monthly_transfer_bytes_used = $2
		WHERE id = $3`, limits.monthlyMinutesUsed, limits.monthlyTransferUsed, userID); err != nil {
		return fmt.Errorf("update user usage: %w", err)
	}

	if usageAtLimit(limits) {
		if _, err := tx.ExecContext(ctx, "UPDATE users SET usage_limit_reached_at = CURRENT_TIMESTAMP WHERE id = $1", userID); err != nil {
			return fmt.Errorf("mark usage limit: %w", err)
		}
		rows, err := tx.QueryContext(ctx, `
			UPDATE tunnels
			SET status = 'stopped', stopped_at = CURRENT_TIMESTAMP
			WHERE user_id = $1 AND status = 'active'
			RETURNING id`, userID)
		if err != nil {
			return fmt.Errorf("stop tunnels for usage limit: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var tunnelID string
			if err := rows.Scan(&tunnelID); err != nil {
				return fmt.Errorf("read stopped tunnel: %w", err)
			}
			stops[tunnelID] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate stopped tunnels: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit usage update: %w", err)
	}
	return nil
}

func lockAndRefreshUserUsage(ctx context.Context, tx *sql.Tx, userID string, now time.Time) (userLimits, error) {
	var (
		limits      userLimits
		periodStart time.Time
	)
	err := tx.QueryRowContext(ctx, `
		SELECT plans.max_active_tunnels, plans.monthly_minutes, plans.monthly_transfer_bytes,
			users.monthly_minutes_used, users.monthly_transfer_bytes_used, users.usage_period_started_at
		FROM users
		JOIN plans ON plans.id = users.plan_id
		WHERE users.id = $1
		FOR UPDATE OF users`, userID).Scan(
		&limits.maxActiveTunnels, &limits.monthlyMinutesLimit, &limits.monthlyTransferLimit,
		&limits.monthlyMinutesUsed, &limits.monthlyTransferUsed, &periodStart,
	)
	if err != nil {
		return userLimits{}, fmt.Errorf("lock user usage: %w", err)
	}
	if !now.Before(periodStart.AddDate(0, 1, 0)) {
		if _, err := tx.ExecContext(ctx, `
			UPDATE users
			SET usage_period_started_at = $1, monthly_minutes_used = 0,
				monthly_transfer_bytes_used = 0, usage_limit_reached_at = NULL
			WHERE id = $2`, now, userID); err != nil {
			return userLimits{}, fmt.Errorf("reset monthly usage: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE tunnels
			SET current_period_minutes = 0, current_period_transfer_bytes = 0
			WHERE user_id = $1`, userID); err != nil {
			return userLimits{}, fmt.Errorf("reset tunnel usage: %w", err)
		}
		limits.monthlyMinutesUsed = 0
		limits.monthlyTransferUsed = 0
	}
	return limits, nil
}

func usageAtLimit(limits userLimits) bool {
	return limits.monthlyMinutesLimit.Valid && limits.monthlyMinutesUsed >= limits.monthlyMinutesLimit.Int64 ||
		limits.monthlyTransferLimit.Valid && limits.monthlyTransferUsed >= limits.monthlyTransferLimit.Int64
}

func validateInput(input CreateInput) (string, net.IP, int, error) {
	protocol := strings.ToLower(strings.TrimSpace(input.Protocol))
	if protocol != "tcp" && protocol != "udp" {
		return "", nil, 0, ErrInvalidInput
	}
	host, portText, err := net.SplitHostPort(strings.TrimSpace(input.InternalAddress))
	if err != nil {
		return "", nil, 0, ErrInvalidInput
	}
	ip := net.ParseIP(host)
	port, err := strconv.Atoi(portText)
	if ip == nil || err != nil || port < 1 || port > 65535 {
		return "", nil, 0, ErrInvalidInput
	}
	return protocol, ip, port, nil
}

func secureToken(byteLength int) (string, error) {
	bytes := make([]byte, byteLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func nullInt64Pointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}

// HTTPProvisioner calls the authenticated tunnel-server management API.
type HTTPProvisioner struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewHTTPProvisionerFromEnv() (*HTTPProvisioner, error) {
	baseURL := strings.TrimRight(os.Getenv("TUNNEL_SERVER_URL"), "/")
	token := os.Getenv("TUNNEL_SERVER_API_TOKEN")
	if baseURL == "" || token == "" {
		return nil, errors.New("TUNNEL_SERVER_URL and TUNNEL_SERVER_API_TOKEN are required")
	}
	return &HTTPProvisioner{baseURL: baseURL, token: token, client: &http.Client{Timeout: 10 * time.Second}}, nil
}

func (p *HTTPProvisioner) Provision(ctx context.Context, request ProvisionRequest) (ProvisionResponse, error) {
	var response ProvisionResponse
	if err := p.doJSON(ctx, http.MethodPost, "/v1/tunnels", request, &response); err != nil {
		return ProvisionResponse{}, err
	}
	return response, nil
}

func (p *HTTPProvisioner) Deprovision(ctx context.Context, tunnelID string) error {
	return p.doJSON(ctx, http.MethodDelete, "/v1/tunnels/"+tunnelID, nil, nil)
}

func (p *HTTPProvisioner) doJSON(ctx context.Context, method, path string, body any, destination any) error {
	var reader *strings.Reader
	if body == nil {
		reader = strings.NewReader("")
	} else {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = strings.NewReader(string(encoded))
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("tunnel server returned %s", resp.Status)
	}
	if destination != nil {
		if err := json.NewDecoder(resp.Body).Decode(destination); err != nil {
			return err
		}
	}
	return nil
}
