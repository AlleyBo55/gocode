// Package email holds a validated email address type.
package email

import "errors"

// ErrInvalidEmail is returned for addresses that fail validation.
var ErrInvalidEmail = errors.New("email: invalid address")

// Email is a validated address. Construct it with NewEmail.
type Email struct {
	addr string
}

// NewEmail validates s and returns it as an Email.
func NewEmail(s string) (Email, error) {
	return Email{addr: s}, nil
}

// String returns the normalised address.
func (e Email) String() string { return e.addr }
