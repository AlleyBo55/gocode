package calc

// SumAll returns the total of nums using a Calc.
func SumAll(nums ...int) int {
	c := NewCalc()
	for _, n := range nums {
		c.Add(n)
	}
	return c.Total()
}

// Apply runs fn against a fresh Calc and returns the total.
func Apply(fn func(*Calc)) int {
	c := NewCalc()
	fn(c)
	return c.Total()
}
