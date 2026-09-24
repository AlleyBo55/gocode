// Package config loads a key=value configuration file.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrEmpty is returned when the file has no key=value lines.
var ErrEmpty = errors.New("config file is empty")

// LoadConfig reads path and returns its key=value pairs.
func LoadConfig(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	defer f.Close()

	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("load config: %w", ErrEmpty)
	}
	return out, nil
}
