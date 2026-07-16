// Command server is the entry point for the Mole VPS server.
// It parses command-line flags, constructs the orchestrator engine, and
// handles graceful shutdown on SIGINT/SIGTERM.
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"Mole/server/internal/config"
	"Mole/server/pkg/orchestrator"
)

func main() {
	cfg := config.Config{}

	flag.IntVar(&cfg.ControlPort, "control-port", 9000,
		"TCP port the server listens on for Mole client control connections")
	flag.IntVar(&cfg.PublicPort, "public-port", 8000,
		"TCP/UDP port exposed to the public internet for end-user traffic")
	flag.StringVar(&cfg.Secret, "secret", "",
		"Shared secret token clients must present to use the control port")
	flag.Parse()

	if cfg.Secret == "" {
		log.Fatal("[server] --secret is required")
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.Printf("[server] starting Mole server — control-port=%d public-port=%d",
		cfg.ControlPort, cfg.PublicPort)

	engine := orchestrator.New(cfg.ControlPort, cfg.PublicPort, cfg.Secret)

	// Catch OS signals for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("[server] received signal %s — shutting down", sig)
		engine.Stop()
	}()

	if err := engine.Run(); err != nil {
		log.Fatalf("[server] engine error: %v", err)
	}

	log.Printf("[server] shutdown complete")
	os.Exit(0)
}
