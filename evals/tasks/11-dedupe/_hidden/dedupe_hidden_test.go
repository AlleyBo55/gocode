package slices

import (
	"reflect"
	"testing"
)

func TestDedupe(t *testing.T) {
	cases := []struct {
		in, want []string
	}{
		{nil, []string{}},
		{[]string{}, []string{}},
		{[]string{"a"}, []string{"a"}},
		{[]string{"a", "b", "a", "c", "b"}, []string{"a", "b", "c"}},
		{[]string{"x", "x", "x"}, []string{"x"}},
		{[]string{"A", "a"}, []string{"A", "a"}},
		{[]string{"", "", "z"}, []string{"", "z"}},
	}
	for _, c := range cases {
		got := Dedupe(c.in)
		if got == nil {
			t.Errorf("Dedupe(%v) returned nil, want a non-nil slice", c.in)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Dedupe(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestDedupeDoesNotMutateInput(t *testing.T) {
	in := []string{"b", "a", "b"}
	_ = Dedupe(in)
	if !reflect.DeepEqual(in, []string{"b", "a", "b"}) {
		t.Errorf("input was modified: %v", in)
	}
}
