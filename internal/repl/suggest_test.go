package repl

import "testing"

func TestSuggestCommandNearMisses(t *testing.T) {
	// Clear single-edit typos should always produce a suggestion.
	for _, typo := range []string{"stauts", "hepl", "commt", "plam", "docter", "ultraplna"} {
		if _, ok := suggestCommand(typo); !ok {
			t.Errorf("suggestCommand(%q): expected a suggestion", typo)
		}
	}
	if got, ok := suggestCommand("status"); !ok || got != "status" {
		t.Errorf("exact match: got %q ok=%v", got, ok)
	}
	// Gibberish far from every command should not be offered.
	if _, ok := suggestCommand("zxqwvk"); ok {
		t.Error("suggestCommand(gibberish): expected no suggestion")
	}
}

func TestIsLoneCommandToken(t *testing.T) {
	lone := []string{"/status", "/x", "/init-deep"}
	notLone := []string{"/etc/passwd is the file", "/path/to x", "hello", "/", "/has space"}
	for _, s := range lone {
		if !isLoneCommandToken(s) {
			t.Errorf("isLoneCommandToken(%q) = false, want true", s)
		}
	}
	for _, s := range notLone {
		if isLoneCommandToken(s) {
			t.Errorf("isLoneCommandToken(%q) = true, want false", s)
		}
	}
}
