// Package calc is a tiny accumulator.
package calc

// Calc accumulates a running total.
type Calc struct {
	total int
}

// NewCalc returns a Calc starting at zero.
func NewCalc() *Calc {
	return &Calc{}
}

// Add adds n to the running total and returns the receiver for chaining.
func (c *Calc) Add(n int) *Calc {
	c.total += n
	return c
}

// Total returns the running total.
func (c *Calc) Total() int {
	return c.total
}
