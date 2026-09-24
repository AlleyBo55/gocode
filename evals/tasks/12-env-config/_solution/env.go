// Package envconfig reads the service configuration from the environment.
package envconfig

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config is the service configuration.
type Config struct {
	Port  int
	Host  string
	Debug bool
}

// Load reads APP_PORT, APP_HOST and APP_DEBUG.
func Load() (Config, error) {
	cfg := Config{Port: 8080, Host: "127.0.0.1"}
	if v := os.Getenv("APP_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("APP_PORT %q is not an integer: %w", v, err)
		}
		if port < 1 || port > 65535 {
			return Config{}, fmt.Errorf("APP_PORT %d is out of range 1..65535", port)
		}
		cfg.Port = port
	}
	if v := os.Getenv("APP_HOST"); v != "" {
		cfg.Host = v
	}
	if v := os.Getenv("APP_DEBUG"); strings.EqualFold(v, "true") || v == "1" {
		cfg.Debug = true
	}
	return cfg, nil
}
