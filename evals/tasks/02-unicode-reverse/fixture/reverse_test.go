package text

import "testing"

func TestReverseASCII(t *testing.T) {
	if got := Reverse("hello"); got != "olleh" {
		t.Errorf("Reverse(hello) = %q", got)
	}
	if got := Reverse(""); got != "" {
		t.Errorf("Reverse(\"\") = %q", got)
	}
}
