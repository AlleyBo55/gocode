// Package sumcsv sums comma-separated integers.
package sumcsv

import (
	"fmt"
	"strconv"
	"strings"
)

// SumCSV parses a comma-separated list of integers and returns their sum.
// It returns an error if any field is not an integer.
func SumCSV(s string) (int, error) {
	total := 0
	for _, field := range strings.Split(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil {
			return 0, fmt.Errorf("sumcsv: field %q is not an integer: %w", field, err)
		}
		total += n
	}
	return total, nil
}
