package counter

import (
	"sync"
	"testing"
)

func TestCounterConcurrentIncrements(t *testing.T) {
	c := NewCounter()
	var wg sync.WaitGroup
	for g := 0; g < 50; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				c.Inc()
			}
		}()
	}
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Add(5)
			_ = c.Value()
		}()
	}
	wg.Wait()
	if got := c.Value(); got != 50*200+10*5 {
		t.Errorf("Value = %d, want %d", got, 50*200+10*5)
	}
}

func TestCounterSequential(t *testing.T) {
	c := NewCounter()
	c.Inc()
	c.Add(-3)
	if c.Value() != -2 {
		t.Errorf("Value = %d, want -2", c.Value())
	}
}
