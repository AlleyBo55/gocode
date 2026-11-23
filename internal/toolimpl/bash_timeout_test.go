package toolimpl

import (
	"strings"
	"testing"
	"time"
)

// A long-running child that keeps stdout open must not hold the tool open
// after timeout_ms.
func TestBashToolReturnsAtTimeoutWhenChildHoldsOutput(t *testing.T) {
	for _, command := range []string{
		"cd . && sleep 30",
		"sleep 30 & echo started",
	} {
		start := time.Now()
		res := (&BashTool{}).Execute(map[string]interface{}{
			"command":    command,
			"timeout_ms": float64(500),
		})
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("%q: tool returned after %v, want about 500ms", command, elapsed)
		}
		if res.Success || !strings.Contains(res.Error, "timed out") {
			t.Fatalf("%q: want a timeout error, got %+v", command, res)
		}
	}
}
