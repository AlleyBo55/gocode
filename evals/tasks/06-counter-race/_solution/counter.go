// Package counter provides a shared counter.
package counter

import "sync/atomic"

// Counter counts events. It is safe for concurrent use.
type Counter struct {
	n atomic.Int64
}

// NewCounter returns a Counter at zero.
func NewCounter() *Counter { return &Counter{} }

// Inc adds one.
func (c *Counter) Inc() { c.n.Add(1) }

// Add adds delta.
func (c *Counter) Add(delta int64) { c.n.Add(delta) }

// Value returns the current count.
func (c *Counter) Value() int64 { return c.n.Load() }
