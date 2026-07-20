// Command server is the entry point for the Mole VPS server.
// It parses command-line flags, constructs the orchestrator engine, and
// handles graceful shutdown on SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"Mole/server/internal/config"
	"Mole/server/pkg/orchestrator"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[server] load configuration: %v", err)
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.Printf("[server] starting Mole server — control-port=%d public-ports=%d-%d",
		cfg.ControlPort, cfg.PortMin, cfg.PortMax)

	engine, err := orchestrator.New(orchestrator.Config{
		ControlPort: cfg.ControlPort,
		PortMin:     cfg.PortMin,
		PortMax:     cfg.PortMax,
		PublicHost:  cfg.PublicHost,
	})
	if err != nil {
		log.Fatalf("[server] invalid configuration: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	orchestrator.StartUsageSync(ctx, engine, cfg.UsageSyncURL, cfg.UsageSyncToken, 5*time.Minute)

	managementServer := &http.Server{Addr: cfg.APIListen, Handler: orchestrator.NewManagementAPI(engine, cfg.APIToken), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("[server] management API listening on %s", cfg.APIListen)
		if err := managementServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[server] management API error: %v", err)
			engine.Stop()
		}
	}()

	// Catch OS signals for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("[server] received signal %s — shutting down", sig)
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = managementServer.Shutdown(shutdownCtx)
		engine.Stop()
	}()

	if err := engine.Run(); err != nil {
		log.Fatalf("[server] engine error: %v", err)
	}

	log.Printf("[server] shutdown complete")
}
