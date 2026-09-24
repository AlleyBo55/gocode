// Package counter provides a shared counter.
package counter

// Counter counts events. It is shared between goroutines.
type Counter struct {
	n int64
}

// NewCounter returns a Counter at zero.
func NewCounter() *Counter { return &Counter{} }

// Inc adds one.
func (c *Counter) Inc() { c.n++ }

// Add adds delta.
func (c *Counter) Add(delta int64) { c.n += delta }

// Value returns the current count.
func (c *Counter) Value() int64 { return c.n }
