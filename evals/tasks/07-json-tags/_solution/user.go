// Package user defines the API's user record.
package user

import "time"

// User is a user record as returned by the API.
type User struct {
	ID        int       `json:"id"`
	FullName  string    `json:"full_name"`
	Email     string    `json:"email,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	IsAdmin   bool      `json:"is_admin"`
}
