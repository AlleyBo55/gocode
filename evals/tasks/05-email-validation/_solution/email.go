// Package email holds a validated email address type.
package email

import (
	"errors"
	"strings"
)

// ErrInvalidEmail is returned for addresses that fail validation.
var ErrInvalidEmail = errors.New("email: invalid address")

// Email is a validated address. Construct it with NewEmail.
type Email struct {
	addr string
}

// NewEmail validates s and returns it as an Email.
func NewEmail(s string) (Email, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.Count(s, "@") != 1 {
		return Email{}, ErrInvalidEmail
	}
	local, domain, _ := strings.Cut(s, "@")
	if local == "" || domain == "" || !strings.Contains(domain, ".") {
		return Email{}, ErrInvalidEmail
	}
	return Email{addr: local + "@" + strings.ToLower(domain)}, nil
}

// String returns the normalised address.
func (e Email) String() string { return e.addr }
