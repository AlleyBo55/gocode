package calc

import "fmt"

// Report describes the state of a Calculator.
func Report(c *Calculator) string {
	return fmt.Sprintf("total=%d", c.Total())
}
