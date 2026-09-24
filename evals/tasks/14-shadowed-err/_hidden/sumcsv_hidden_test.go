package sumcsv

import (
	"strings"
	"testing"
)

func TestSumCSVValid(t *testing.T) {
	cases := map[string]int{
		"1,2,3":     6,
		" 10 , 20 ": 30,
		"-5,5":      0,
		"42":        42,
	}
	for in, want := range cases {
		got, err := SumCSV(in)
		if err != nil {
			t.Errorf("SumCSV(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("SumCSV(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestSumCSVRejectsBadFields(t *testing.T) {
	for in, bad := range map[string]string{"1,2,x": "x", "1,,2": "", "3.5,1": "3.5", "abc": "abc"} {
		_, err := SumCSV(in)
		if err == nil {
			t.Errorf("SumCSV(%q) should fail", in)
			continue
		}
		if bad != "" && !strings.Contains(err.Error(), bad) {
			t.Errorf("SumCSV(%q) error %q should mention %q", in, err, bad)
		}
	}
}
