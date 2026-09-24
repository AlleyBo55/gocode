package email

import (
	"errors"
	"testing"
)

func TestNewEmailAccepts(t *testing.T) {
	cases := map[string]string{
		"a@b.co":                  "a@b.co",
		"  User@Example.COM  ":    "User@example.com",
		"first.last+tag@Sub.D.io": "first.last+tag@sub.d.io",
	}
	for in, want := range cases {
		e, err := NewEmail(in)
		if err != nil {
			t.Errorf("NewEmail(%q) error: %v", in, err)
			continue
		}
		if e.String() != want {
			t.Errorf("NewEmail(%q).String() = %q, want %q", in, e.String(), want)
		}
	}
}

func TestNewEmailRejects(t *testing.T) {
	for _, in := range []string{"", "   ", "nobody", "@example.com", "user@", "a@b@c.com", "user@localhost", "user@nodot"} {
		if _, err := NewEmail(in); !errors.Is(err, ErrInvalidEmail) {
			t.Errorf("NewEmail(%q) = %v, want ErrInvalidEmail", in, err)
		}
	}
}
