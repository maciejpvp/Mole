package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ControlPort    int
	PortMin        int
	PortMax        int
	PublicHost     string
	APIListen      string
	APIToken       string
	UsageSyncURL   string
	UsageSyncToken string
}

// Load reads .env when present, then loads the relay configuration from the
// environment. Environment variables supplied by the process take precedence
// over .env, which keeps container and production deployments configurable.
func Load() (Config, error) {
	if err := loadDotEnv(".env"); err != nil {
		return Config{}, err
	}

	controlPort, err := envInt("MOLE_CONTROL_PORT", 9000)
	if err != nil {
		return Config{}, err
	}
	portMin, err := envInt("MOLE_TUNNEL_PORT_MIN", 10000)
	if err != nil {
		return Config{}, err
	}
	portMax, err := envInt("MOLE_TUNNEL_PORT_MAX", 10100)
	if err != nil {
		return Config{}, err
	}
	config := Config{
		ControlPort:    controlPort,
		PortMin:        portMin,
		PortMax:        portMax,
		PublicHost:     strings.TrimSpace(os.Getenv("MOLE_PUBLIC_HOST")),
		APIListen:      envString("MOLE_API_LISTEN", ":9001"),
		APIToken:       os.Getenv("MOLE_SERVER_API_TOKEN"),
		UsageSyncURL:   strings.TrimSpace(os.Getenv("MOLE_CONTROL_PLANE_URL")),
		UsageSyncToken: os.Getenv("MOLE_USAGE_SYNC_TOKEN"),
	}
	if config.PublicHost == "" || config.APIToken == "" || config.UsageSyncURL == "" || config.UsageSyncToken == "" {
		return Config{}, errors.New("MOLE_PUBLIC_HOST, MOLE_SERVER_API_TOKEN, MOLE_CONTROL_PLANE_URL, and MOLE_USAGE_SYNC_TOKEN are required")
	}
	return config, nil
}

func envInt(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, err)
	}
	return parsed, nil
}

func envString(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func loadDotEnv(path string) error {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	for lineNumber, line := range strings.Split(string(contents), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		name, value, found := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !found || name == "" {
			return fmt.Errorf("parse %s line %d", path, lineNumber+1)
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			unquoted, err := strconv.Unquote(value)
			if err != nil {
				return fmt.Errorf("parse %s line %d: %w", path, lineNumber+1, err)
			}
			value = unquoted
		}
		if _, exists := os.LookupEnv(name); !exists {
			if err := os.Setenv(name, value); err != nil {
				return fmt.Errorf("set %s: %w", name, err)
			}
		}
	}
	return nil
}
