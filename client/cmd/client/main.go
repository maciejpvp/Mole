package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"Mole/client/pkg/client"
)

func main() {
	var (
		serverAddr  string
		localTarget string
		localProto  string
		secret      string
		debug       bool
	)

	flag.StringVar(&serverAddr, "server", "", "VPS address (host:port)")
	flag.StringVar(&localTarget, "target", "", "Local target (host:port)")
	flag.StringVar(&localProto, "proto", "", "Protocol: tcp or udp")
	flag.StringVar(&secret, "secret", "", "Shared secret token to authenticate with the server")
	flag.BoolVar(&debug, "debug", false, "Enable verbose debug logs")
	flag.Parse()

	if localProto != "tcp" && localProto != "udp" {
		log.Fatalf("[client] --proto must be 'tcp' or 'udp', got: %q", localProto)
	}
	if serverAddr == "" {
		log.Fatalf("[client] --server is required")
	}
	if localTarget == "" {
		log.Fatalf("[client] --target is required")
	}
	if secret == "" {
		log.Fatalf("[client] --secret is required")
	}

	client.Debug = debug

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	if debug {
		log.Printf("[client] starting Mole client — server=%s proto=%s local=%s",
			serverAddr, localProto, localTarget)
	} else {
		log.Printf("[client] Mole client is running...")
	}

	agent := client.New(serverAddr, localTarget, localProto, secret)

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
