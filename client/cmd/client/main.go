package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"Mole/client/pkg/client"
)

func main() {
	var (
		token   string
		moleURL string
		debug   bool
	)

	flag.StringVar(&token, "token", "", "Tunnel token returned by the control plane")
	flag.StringVar(&moleURL, "mole-url", "", "Control-plane base URL (for example http://127.0.0.1:8080)")
	flag.BoolVar(&debug, "debug", false, "Enable verbose debug logs")
	flag.Parse()

	if token == "" {
		log.Fatalf("[client] --token is required")
	}
	if moleURL == "" {
		log.Fatal("[client] --mole-url is required")
	}
	tunnelConfig, err := client.FetchTunnelConfig(context.Background(), moleURL, token)
	if err != nil {
		log.Fatalf("[client] fetch tunnel configuration: %v", err)
	}

	client.Debug = debug

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	if debug {
		log.Printf("[client] starting Mole client — server=%s proto=%s local=%s",
			tunnelConfig.ServerAddress, tunnelConfig.Protocol, tunnelConfig.InternalAddress)
	} else {
		log.Printf("[client] Mole client is running...")
	}

	agent := client.New(tunnelConfig.ServerAddress, tunnelConfig.InternalAddress, tunnelConfig.Protocol, token)

	// Catch OS signals for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		if debug {
			log.Printf("[client] received signal %s — shutting down", sig)
		}
		agent.Stop()
	}()

	// Catch Ctrl+D (EOF on stdin) to stop the client.
	go func() {
		buf := make([]byte, 1024)
		for {
			_, err := os.Stdin.Read(buf)
			if err != nil {
				if debug {
					log.Printf("[client] stdin closed (Ctrl+D) — shutting down")
				}
				agent.Stop()
				return
			}
		}
	}()

	agent.Run()

	agent.Wait()
	if debug {
		log.Printf("[client] shutdown complete")
	}
	os.Exit(0)
}
