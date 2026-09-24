package text

import "testing"

func TestReverseUnicode(t *testing.T) {
	cases := map[string]string{
		"héllo": "olléh",
		"日本語":   "語本日",
		"a😀b":   "b😀a",
		"abc":   "cba",
		"x":     "x",
	}
	for in, want := range cases {
		if got := Reverse(in); got != want {
			t.Errorf("Reverse(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReverseIsAnInvolution(t *testing.T) {
	for _, s := range []string{"héllo wörld", "日本語テキスト", "plain"} {
		if got := Reverse(Reverse(s)); got != s {
			t.Errorf("Reverse(Reverse(%q)) = %q", s, got)
		}
	}
}
