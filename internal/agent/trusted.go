package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// TrustedToolStore manages a persistent list of trusted tool patterns.
// Patterns can be:
//   - Exact tool name: "BashTool" — trusts this tool with any params
//   - Tool + prefix: "BashTool:git *" — trusts BashTool when the command starts
//     with the word "git". A compound command is trusted only when every part
//     of it is: "git add . && git commit" is, "git status; rm -rf /" is not.
//     For search tools the prefix is matched against "pattern"; for file tools
//     against the directory of "path".
//   - Wildcard: "*" — trusts everything (equivalent to --dangerously-skip-permissions)
type TrustedToolStore struct {
	mu       sync.RWMutex
	patterns []string
	path     string
}

// NewTrustedToolStore creates a store backed by a JSON file.
func NewTrustedToolStore(path string) *TrustedToolStore {
	if path == "" {
		path = filepath.Join(".gocode", "trusted_tools.json")
	}
	return &TrustedToolStore{path: path}
}

// Load reads trusted patterns from disk.
func (s *TrustedToolStore) Load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Unmarshal(data, &s.patterns)
}

// Save writes trusted patterns to disk.
func (s *TrustedToolStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.patterns, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// Add adds a trusted pattern and persists to disk.
func (s *TrustedToolStore) Add(pattern string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Don't add duplicates
	for _, p := range s.patterns {
		if p == pattern {
			return
		}
	}
	s.patterns = append(s.patterns, pattern)
}

// IsTrusted checks if a tool invocation matches any trusted pattern.
//
// input is the tool's JSON parameter object as the runtime hands it to
// Authorize. A "Tool:prefix" pattern is matched against the parameter the
// prefix was taken from when the user chose to trust it: the command's leading
// words, the search pattern, or the path's directory. It is never matched
// against the raw JSON text. The previous substring match meant "BashTool:ls *"
// approved `rm -rf ./tools` and "BashTool:git *" approved any command whose
// description mentioned git.
func (s *TrustedToolStore) IsTrusted(toolName string, input string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	toolLower := strings.ToLower(toolName)
	var prefixes []string

	for _, pattern := range s.patterns {
		patLower := strings.ToLower(strings.TrimSpace(pattern))

		// Global wildcard — trust everything
		if patLower == "*" {
			return true
		}

		pTool, pPrefix, hasPrefix := strings.Cut(patLower, ":")
		if strings.TrimSpace(pTool) != toolLower {
			continue
		}

		// Bare tool name — trust this tool with any params
		if !hasPrefix {
			return true
		}

		// "Tool:git *" and "Tool:git" both mean "starts with git". A pattern
		// with nothing after the colon is malformed and trusts nothing; the
		// bare tool name is how "any params" is spelled.
		pPrefix = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(pPrefix), " *"))
		if pPrefix == "" {
			continue
		}
		prefixes = append(prefixes, pPrefix)
	}
	if len(prefixes) == 0 {
		return false
	}
	return parseTrustFields(input).trustedBy(prefixes)
}

// trustFields holds the parameters a trust prefix can be matched against,
// lowercased to match the lowercased patterns.
type trustFields struct {
	command string
	pattern string
	path    string // cleaned, so "src/../etc" cannot pass as "src"
	raw     string // set only when input is not a JSON object
}

func parseTrustFields(input string) *trustFields {
	f := &trustFields{}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(input), &m); err != nil || m == nil {
		// Not the runtime's JSON object. Treat the whole string as a command
		// so a caller passing a raw command line still gets prefix semantics.
		f.raw = strings.ToLower(strings.TrimSpace(input))
		return f
	}
	if v, ok := m["command"].(string); ok {
		f.command = strings.ToLower(strings.TrimSpace(v))
	}
	if v, ok := m["pattern"].(string); ok {
		f.pattern = strings.ToLower(v)
	}
	if v, ok := m["path"].(string); ok && v != "" {
		f.path = strings.ToLower(filepath.Clean(v))
	}
	return f
}

// trustedBy reports whether the parameters are covered by the tool's trusted
// prefixes. A command line must be covered in full (see commandTrusted); a
// search pattern or path needs any one prefix to match.
func (f *trustFields) trustedBy(prefixes []string) bool {
	if f.raw != "" {
		return commandTrusted(f.raw, prefixes)
	}
	if f.command != "" && commandTrusted(f.command, prefixes) {
		return true
	}
	for _, p := range prefixes {
		if f.pattern != "" && strings.HasPrefix(f.pattern, p) {
			return true
		}
		if f.path != "" && dirPrefix(f.path, p) {
			return true
		}
	}
	return false
}

// commandTrusted reports whether every command on a shell line starts with a
// trusted prefix. The line is split where the shell would start a new
// command (; && || | & and newlines) and each piece must match on its own,
// so "git status; rm -rf /" is not covered by "git *" and "git log | head"
// needs "head *" as well. Command substitution runs something the prefix
// does not describe, and redirection writes somewhere it does not describe;
// neither is auto-trusted, except the redirections that only discard output
// or join file descriptors.
func commandTrusted(cmd string, prefixes []string) bool {
	if strings.Contains(cmd, "`") || strings.Contains(cmd, "$(") {
		return false
	}
	for _, seg := range splitShellCommands(cmd) {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue // "git status;" or "cmd &": nothing runs in the empty piece
		}
		if hasWritingRedirect(seg) {
			return false
		}
		matched := false
		for _, p := range prefixes {
			if wordPrefix(seg, p) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// splitShellCommands splits on the operators that separate commands. A "&"
// that is part of a redirection ("2>&1", "&>/dev/null") is left alone.
func splitShellCommands(cmd string) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		out = append(out, cur.String())
		cur.Reset()
	}
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case c == ';' || c == '\n':
			flush()
		case c == '|':
			if i+1 < len(cmd) && cmd[i+1] == '|' {
				i++
			}
			flush()
		case c == '&':
			prevRedirect := i > 0 && cmd[i-1] == '>'
			nextRedirect := i+1 < len(cmd) && cmd[i+1] == '>'
			if prevRedirect || nextRedirect {
				cur.WriteByte(c)
				continue
			}
			if i+1 < len(cmd) && cmd[i+1] == '&' {
				i++
			}
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return out
}

// redirectRe matches an output redirection and its target: ">file",
// ">> log", "2>&1", "&>/dev/null".
var redirectRe = regexp.MustCompile(`(\d?&?>>?&?)\s*(\S+)`)

// hasWritingRedirect reports whether a command segment redirects output to a
// file. Sending output to /dev/null or to another file descriptor is not
// writing a file and stays trusted.
func hasWritingRedirect(seg string) bool {
	for _, m := range redirectRe.FindAllStringSubmatch(seg, -1) {
		op, target := m[1], m[2]
		if target == "/dev/null" {
			continue
		}
		if strings.HasSuffix(op, "&") && isDigits(target) {
			continue // 2>&1, 1>&2
		}
		return true
	}
	return false
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// wordPrefix reports whether s starts with prefix on a word boundary, so
// "git" matches "git status" but not "gitk".
func wordPrefix(s, prefix string) bool {
	if !strings.HasPrefix(s, prefix) {
		return false
	}
	if len(s) == len(prefix) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(s[len(prefix):])
	return unicode.IsSpace(r)
}

// dirPrefix reports whether path is prefix or lies under it, so "src" matches
// "src/main.go" but not "srcs/main.go".
func dirPrefix(path, prefix string) bool {
	prefix = filepath.Clean(prefix)
	return path == prefix || strings.HasPrefix(path, prefix+string(filepath.Separator))
}

// List returns all trusted patterns.
func (s *TrustedToolStore) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.patterns))
	copy(out, s.patterns)
	return out
}

// Remove removes a trusted pattern by index.
func (s *TrustedToolStore) Remove(index int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.patterns) {
		return false
	}
	s.patterns = append(s.patterns[:index], s.patterns[index+1:]...)
	return true
}
