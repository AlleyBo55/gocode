// Package stats has small numeric helpers.
package stats

// Sum returns the total of nums. An empty or nil slice sums to 0.
func Sum(nums []int) int {
	total := 0
	for _, n := range nums {
		total += n
	}
	return total
}

// Mean returns the arithmetic mean of nums, or 0 for an empty slice.
func Mean(nums []int) float64 {
	if len(nums) == 0 {
		return 0
	}
	return float64(Sum(nums)) / float64(len(nums))
}
