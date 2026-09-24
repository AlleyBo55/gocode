package calc

import "fmt"

// Report describes the state of a Calc.
func Report(c *Calc) string {
	return fmt.Sprintf("total=%d", c.Total())
}
