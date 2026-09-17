package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/joho/godotenv/autoload"
	"mole-control-plane/internal/admin"
	"mole-control-plane/internal/billing"
	"mole-control-plane/internal/database"
	"mole-control-plane/internal/relay"
	serveradmin "mole-control-plane/internal/server/admin"
	serverauth "mole-control-plane/internal/server/auth"
	serverbilling "mole-control-plane/internal/server/billing"
	"mole-control-plane/internal/server/events"
	servertunnels "mole-control-plane/internal/server/tunnels"
	serverusers "mole-control-plane/internal/server/users"
	"mole-control-plane/internal/tunnel"
	"mole-control-plane/internal/user"
)

// Server composes transport handlers and infrastructure.
type Server struct {
	db           database.Service
	users        *user.Service
	auth         *serverauth.Handler
	billing      *serverbilling.Handler
	admin        *serveradmin.Handler
	tunnels      *servertunnels.Handler
	usersHandler *serverusers.Handler
	events       *events.Handler
}

// Application owns both the HTTP API and the embedded relay listeners.
type Application struct {
	HTTPServer *http.Server
	relay      *relay.Engine
	tunnels    *tunnel.Service
}

func NewApplication() *Application {
	port, _ := strconv.Atoi(os.Getenv("PORT"))
	db := database.New()
	users := user.NewService(db.DB())
	promoteBootstrapAdmin(users)

	relayEngine, setupErr := newRelayFromEnv()
	if setupErr != nil {
		log.Printf("embedded relay is unavailable: %v", setupErr)
	}
	var provisioner tunnel.Provisioner
	if relayEngine != nil {
		provisioner = tunnel.NewRelayProvisioner(relayEngine)
	}
	tunnelService := tunnel.NewService(db.DB(), provisioner)
	broker := events.NewBroker()
	eventHandler := events.NewHandler(users, tunnelService, broker)
	if relayEngine != nil {
		relayEngine.SetCallbacks(relay.Callbacks{
			ApplyUsage: func(ctx context.Context, updates []relay.UsageUpdate) ([]string, error) {
				converted := make([]tunnel.UsageUpdate, len(updates))
				for i, update := range updates {
					converted[i] = tunnel.UsageUpdate{TunnelID: update.TunnelID, ActiveMinutes: update.ActiveMinutes, TransferBytes: update.TransferBytes}
				}
				result, err := tunnelService.ApplyUsage(ctx, converted)
				if err == nil {
					eventHandler.NotifyUserUpdateForUsage(ctx, converted)
				}
				return result.StopTunnelIDs, err
			},
			SetConnectionStatus: func(ctx context.Context, update relay.ConnectionStatusUpdate) error {
				err := tunnelService.SetConnectionStatus(ctx, update.TunnelID, update.Status)
				if err == nil {
					eventHandler.NotifyUserUpdateForTunnel(ctx, update.TunnelID)
				}
				return err
			},
		})
	}

	api := &Server{
		db: db, users: users,
		auth:         serverauth.NewHandler(users),
		billing:      serverbilling.NewHandler(billing.NewService(db.DB(), os.Getenv("STRIPE_SECRET_KEY"), os.Getenv("STRIPE_WEBHOOK_SECRET"))),
		admin:        serveradmin.NewHandler(admin.NewService(db.DB(), provisioner), broker),
		tunnels:      servertunnels.NewHandler(tunnelService, setupErr, eventHandler),
		usersHandler: serverusers.NewHandler(users, tunnelService, eventHandler),
		events:       eventHandler,
	}
	return &Application{
		HTTPServer: &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: api.RegisterRoutes(), IdleTimeout: time.Minute, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second},
		relay:      relayEngine,
		tunnels:    tunnelService,
	}
}

// NewServer remains available to callers that only need the HTTP server.
func NewServer() *http.Server { return NewApplication().HTTPServer }

func (a *Application) Start(ctx context.Context) error {
	if a.relay == nil {
		return nil
	}
	// Rebuild the relay registry before the control listener accepts, so a
	// reconnecting client never races a half-built registry and gets rejected.
	if a.tunnels != nil {
		if err := a.tunnels.Restore(ctx); err != nil {
			log.Printf("restore tunnels: %v", err)
		}
	}
	return a.relay.Start(ctx)
}

func (a *Application) Shutdown(ctx context.Context) error {
	if a.relay != nil {
		a.relay.Stop()
	}
	return a.HTTPServer.Shutdown(ctx)
}

// promoteBootstrapAdmin grants administrator permission to BOOTSTRAP_ADMIN_EMAIL
// at startup, so a fresh deployment can reach the admin API without hand-written
// SQL. Never fatal: a typo must not take the API down. The account may not exist
// yet on a first deploy, in which case user.LoginWithGoogle promotes it at
// sign-in instead.
func promoteBootstrapAdmin(users *user.Service) {
	email := strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_EMAIL"))
	if email == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch err := users.PromoteAdminByEmail(ctx, email); {
	case errors.Is(err, user.ErrAccountUnavailable):
		log.Printf("bootstrap admin: no account for %q yet; it will be promoted on first Google sign-in", email)
	case err != nil:
		log.Printf("bootstrap admin: %v", err)
	default:
		log.Printf("bootstrap admin: %q now has administrator permission", email)
	}
}

func newRelayFromEnv() (*relay.Engine, error) {
	controlPort, err := relayEnvInt("MOLE_CONTROL_PORT", 9000)
	if err != nil {
		return nil, err
	}
	portMin, err := relayEnvInt("MOLE_TUNNEL_PORT_MIN", 10000)
	if err != nil {
		return nil, err
	}
	portMax, err := relayEnvInt("MOLE_TUNNEL_PORT_MAX", 10100)
	if err != nil {
		return nil, err
	}
	globalLimit, err := relayEnvInt64("MOLE_GLOBAL_TRANSFER_LIMIT_BYTES", 5*1024*1024*1024*1024)
	if err != nil {
		return nil, err
	}
	return relay.New(relay.Config{ControlPort: controlPort, PortMin: portMin, PortMax: portMax, PublicHost: strings.TrimSpace(os.Getenv("MOLE_PUBLIC_HOST")), GlobalTransferLimitBytes: globalLimit, GlobalTransferStateDir: relayEnvString("MOLE_SERVER_STATE_DIR", "/var/lib/mole-server")})
}

func relayEnvInt(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	result, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, err)
	}
	return result, nil
}
func relayEnvInt64(name string, fallback int64) (int64, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	result, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a 64-bit integer: %w", name, err)
	}
	if result < 0 {
		return 0, fmt.Errorf("%s must be zero or positive", name)
	}
	return result, nil
}
func relayEnvString(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func (s *Server) HelloWorldHandler(w http.ResponseWriter, r *http.Request) {
	response, _ := json.Marshal(map[string]string{"message": "Hello World"})
	_, _ = w.Write(response)
}
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(s.db.Health())
}
