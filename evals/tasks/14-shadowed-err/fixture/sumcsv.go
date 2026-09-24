// Package sumcsv sums comma-separated integers.
package sumcsv

import (
	"strconv"
	"strings"
)

// SumCSV parses a comma-separated list of integers and returns their sum.
// It returns an error if any field is not an integer.
func SumCSV(s string) (int, error) {
	total := 0
	var err error
	for _, field := range strings.Split(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil {
			continue
		}
		total += n
	}
	return total, err
}
