package words

import (
	"reflect"
	"testing"
)

func TestTopWords(t *testing.T) {
	text := "The cat and the hat. THE cat sat; a hat, a cat!"
	got := TopWords(text, 3)
	want := []WordCount{{"cat", 3}, {"the", 3}, {"a", 2}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("TopWords = %v, want %v", got, want)
	}
}

func TestTopWordsFewerThanN(t *testing.T) {
	got := TopWords("b a b", 10)
	want := []WordCount{{"b", 2}, {"a", 1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("TopWords = %v, want %v", got, want)
	}
}

func TestTopWordsUnicodeLettersAndDigits(t *testing.T) {
	got := TopWords("café café 123 naïve", 5)
	want := []WordCount{{"café", 2}, {"naïve", 1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("TopWords = %v, want %v (digits are separators, not words)", got, want)
	}
}

func TestTopWordsEdgeCases(t *testing.T) {
	for name, got := range map[string][]WordCount{
		"n=0":      TopWords("a b", 0),
		"n<0":      TopWords("a b", -1),
		"empty":    TopWords("", 3),
		"no words": TopWords("123 456 !!!", 3),
	} {
		if got == nil || len(got) != 0 {
			t.Errorf("%s: got %v, want empty non-nil slice", name, got)
		}
	}
}
