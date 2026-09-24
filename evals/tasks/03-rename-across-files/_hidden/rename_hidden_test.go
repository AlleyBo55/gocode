package calc

import "testing"

func TestRenamedTypeAndConstructor(t *testing.T) {
	var c *Calculator = NewCalculator()
	c.Add(2).Add(3)
	if c.Total() != 5 {
		t.Errorf("Total = %d, want 5", c.Total())
	}
}

func TestCallersStillWork(t *testing.T) {
	if got := SumAll(1, 2, 3); got != 6 {
		t.Errorf("SumAll = %d", got)
	}
	if got := Apply(func(c *Calculator) { c.Add(10) }); got != 10 {
		t.Errorf("Apply = %d", got)
	}
	if got := Report(NewCalculator().Add(7)); got != "total=7" {
		t.Errorf("Report = %q", got)
	}
}
