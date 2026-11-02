package rea

import "testing"

func TestNodeSatisfies(t *testing.T) {
	cases := []struct {
		ver  string
		want bool
	}{
		{"v24.22.1", true},  // pinned
		{"v24.22.0", false}, // below ^24.22.1
		{"v24.18.0", false}, // below
		{"v24.30.0", true},  // within ^24
		{"v22.23.4", true},  // ^22.23.4 lower bound
		{"v22.23.3", false}, // below patch
		{"v22.22.0", false}, // below minor
		{"v23.0.0", false},  // gap between ranges
		{"v25.0.0", false},  // gap
		{"v26.12.1", true},  // >=26.12.1
		{"v26.12.0", false}, // below
		{"v28.0.0", true},   // future major
		{"v16.13.0", false}, // old
		{"garbage", false},
	}
	for _, c := range cases {
		if got := NodeSatisfies(c.ver); got != c.want {
			t.Errorf("NodeSatisfies(%q) = %v, want %v", c.ver, got, c.want)
		}
	}
}
