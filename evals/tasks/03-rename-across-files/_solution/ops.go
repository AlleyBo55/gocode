package calc

// SumAll returns the total of nums using a Calculator.
func SumAll(nums ...int) int {
	c := NewCalculator()
	for _, n := range nums {
		c.Add(n)
	}
	return c.Total()
}

// Apply runs fn against a fresh Calculator and returns the total.
func Apply(fn func(*Calculator)) int {
	c := NewCalculator()
	fn(c)
	return c.Total()
}
