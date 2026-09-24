// Package user defines the API's user record.
package user

import "time"

// User is a user record as returned by the API.
type User struct {
	ID        int
	FullName  string
	Email     string
	CreatedAt time.Time
	IsAdmin   bool
}
