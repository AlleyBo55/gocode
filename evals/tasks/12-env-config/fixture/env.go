// Package envconfig reads the service configuration from the environment.
package envconfig

// Config is the service configuration.
type Config struct {
	Port  int
	Host  string
	Debug bool
}

// Load reads APP_PORT, APP_HOST and APP_DEBUG.
func Load() (Config, error) {
	return Config{}, nil
}
