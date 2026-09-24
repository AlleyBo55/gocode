// Package kv defines a small key/value store.
package kv

import "errors"

// ErrNotFound is returned when a key does not exist.
var ErrNotFound = errors.New("kv: key not found")

// ErrNotImplemented marks stubbed methods.
var ErrNotImplemented = errors.New("kv: not implemented")

// Store is a string key/value store.
type Store interface {
	// Get returns the value for key, or ErrNotFound.
	Get(key string) (string, error)
	// Set stores value under key, replacing any existing value.
	Set(key, value string) error
	// Delete removes key. It returns ErrNotFound if the key was absent.
	Delete(key string) error
	// Keys returns every stored key in sorted order.
	Keys() []string
}
