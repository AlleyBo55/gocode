// Package words counts word frequencies.
package words

import (
	"sort"
	"strings"
	"unicode"
)

// WordCount is a word and how often it appeared.
type WordCount struct {
	Word  string
	Count int
}

// TopWords returns the n most frequent words in text.
func TopWords(text string, n int) []WordCount {
	if n <= 0 {
		return []WordCount{}
	}
	counts := map[string]int{}
	for _, w := range strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) }) {
		counts[strings.ToLower(w)]++
	}
	out := make([]WordCount, 0, len(counts))
	for w, c := range counts {
		out = append(out, WordCount{Word: w, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Word < out[j].Word
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}
