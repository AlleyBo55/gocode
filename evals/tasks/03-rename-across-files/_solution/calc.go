// Package calc is a tiny accumulator.
package calc

// Calculator accumulates a running total.
type Calculator struct {
	total int
}

// NewCalculator returns a Calculator starting at zero.
func NewCalculator() *Calculator {
	return &Calculator{}
}

// Add adds n to the running total and returns the receiver for chaining.
func (c *Calculator) Add(n int) *Calculator {
	c.total += n
	return c
}

// Total returns the running total.
func (c *Calculator) Total() int {
	return c.total
}
