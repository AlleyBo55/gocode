// Package rea detects a local REA ("Reverse Engineer Anything") install so
// gocode can drive its reverse-engineering tools over MCP. gocode does not
// vendor REA; it launches an installed REA as a newline-delimited-JSON stdio
// MCP subprocess. The user is responsible for only pointing these tools at
// artifacts they are authorized to analyze.
package rea

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Install describes a resolved REA install gocode can launch.
type Install struct {
	NodePath    string // absolute path to a node binary REA supports
	ScriptPath  string // absolute path to scripts/rea.mjs
	NodeVersion string // e.g. "v24.22.1"
}

// Command returns the command and args to launch REA's MCP server over stdio.
func (i *Install) Command() (string, []string) {
	return i.NodePath, []string{i.ScriptPath, "mcp"}
}

// Detect locates an installed REA and a compatible node binary.
func Detect() (*Install, error) {
	script, err := findScript()
	if err != nil {
		return nil, err
	}
	node, ver, err := findNode()
	if err != nil {
		return nil, err
	}
	return &Install{NodePath: node, ScriptPath: script, NodeVersion: ver}, nil
}

// findScript searches a few sensible locations for scripts/rea.mjs.
func findScript() (string, error) {
	var candidates []string
	if p := os.Getenv("GOCODE_REA_PATH"); p != "" {
		if strings.HasSuffix(p, ".mjs") {
			candidates = append(candidates, p)
		} else {
			candidates = append(candidates, filepath.Join(p, "scripts", "rea.mjs"))
		}
	}
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	candidates = append(candidates,
		filepath.Join(cwd, "rea", "scripts", "rea.mjs"),
		filepath.Join(cwd, "..", "rea", "scripts", "rea.mjs"),
	)
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, "ngoding", "rea", "scripts", "rea.mjs"),
			filepath.Join(home, "rea", "scripts", "rea.mjs"),
		)
	}
	for _, c := range candidates {
		if abs, err := filepath.Abs(c); err == nil {
			if _, statErr := os.Stat(abs); statErr == nil {
				return abs, nil
			}
		}
	}
	return "", fmt.Errorf("REA not found. Set GOCODE_REA_PATH to your rea checkout (the directory that contains scripts/rea.mjs)")
}

// findNode returns the first node binary that satisfies REA's engine range.
func findNode() (string, string, error) {
	var candidates []string
	if p := os.Getenv("GOCODE_REA_NODE"); p != "" {
		candidates = append(candidates, p)
	}
	if home, _ := os.UserHomeDir(); home != "" {
		nvm := filepath.Join(home, ".nvm", "versions", "node")
		if entries, err := os.ReadDir(nvm); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					candidates = append(candidates, filepath.Join(nvm, e.Name(), "bin", "node"))
				}
			}
		}
	}
	if p, err := exec.LookPath("node"); err == nil {
		candidates = append(candidates, p)
	}

	var seenVersions []string
	for _, c := range candidates {
		ver, err := nodeVersion(c)
		if err != nil {
			continue
		}
		seenVersions = append(seenVersions, ver)
		if NodeSatisfies(ver) {
			abs, _ := filepath.Abs(c)
			return abs, ver, nil
		}
	}
	detail := ""
	if len(seenVersions) > 0 {
		detail = fmt.Sprintf(" (found incompatible: %s)", strings.Join(seenVersions, ", "))
	}
	return "", "", fmt.Errorf("no node satisfying REA's engines (^22.23.4 || ^24.22.1 || >=26.12.1) found%s; set GOCODE_REA_NODE to a suitable node binary", detail)
}

func nodeVersion(nodePath string) (string, error) {
	out, err := exec.Command(nodePath, "--version").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// NodeSatisfies reports whether a node version string satisfies REA's engine
// range: ^22.23.4 || ^24.22.1 || >=26.12.1.
func NodeSatisfies(ver string) bool {
	maj, min, pat := parseVer(ver)
	switch {
	case maj == 22:
		return min > 23 || (min == 23 && pat >= 4)
	case maj == 24:
		return min > 22 || (min == 22 && pat >= 1)
	case maj == 26:
		return min > 12 || (min == 12 && pat >= 1)
	case maj > 26:
		return true
	default:
		return false
	}
}

func parseVer(v string) (int, int, int) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.SplitN(v, ".", 3)
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	return atoiSafe(parts[0]), atoiSafe(parts[1]), atoiSafe(parts[2])
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}
