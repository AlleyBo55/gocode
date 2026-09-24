package toolimpl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func numberedLines(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "line %06d\n", i)
	}
	return b.String()
}

func TestTruncateOutputLeavesSmallOutputAlone(t *testing.T) {
	small := numberedLines(100)
	if got := TruncateOutput("bashtool", small); got != small {
		t.Error("output under the cap must be returned byte for byte")
	}
	exact := strings.Repeat("x", MaxToolOutputBytes)
	if got := TruncateOutput("bashtool", exact); got != exact {
		t.Error("output exactly at the cap must not be truncated")
	}
}

func TestTruncateOutputKeepsHeadAndTailOnLineBoundaries(t *testing.T) {
	big := numberedLines(20_000) // 220 KB
	got := TruncateOutput("bashtool", big)

	if len(got) > MaxToolOutputBytes+400 {
		t.Errorf("truncated output is %d bytes, cap is %d plus a marker", len(got), MaxToolOutputBytes)
	}
	if !strings.HasPrefix(got, "line 000001\n") {
		t.Errorf("head lost: %q", got[:40])
	}
	if !strings.HasSuffix(got, "line 020000\n") {
		t.Errorf("tail lost: %q", got[len(got)-40:])
	}
	// Cuts land on line boundaries, so no half-line garbage on either side
	// of the marker.
	markerAt := strings.Index(got, "[gocode: output truncated.")
	if markerAt < 0 {
		t.Fatal("no marker")
	}
	if got[markerAt-1] != '\n' {
		t.Errorf("head does not end at a line boundary: %q", got[markerAt-20:markerAt])
	}
	after := got[strings.Index(got[markerAt:], "]\n")+markerAt+2:]
	if !strings.HasPrefix(after, "line ") {
		t.Errorf("tail does not start at a line boundary: %q", after[:20])
	}
	for _, want := range []string{"bytes (", "lines) omitted", "showing the first", "Narrow the command"} {
		if !strings.Contains(got, want) {
			t.Errorf("marker should say %q:\n%s", want, got[markerAt:markerAt+300])
		}
	}
}

func TestTruncateOutputHintIsPerTool(t *testing.T) {
	big := strings.Repeat("y", MaxToolOutputBytes+1)
	if got := TruncateOutput("FileReadTool", big); !strings.Contains(got, "start_line and end_line") {
		t.Error("file read hint missing (tool names are case-insensitive)")
	}
	if got := TruncateOutput("some_plugin_tool", big); !strings.Contains(got, "output truncated") || strings.Contains(got, "Narrow the command") {
		t.Error("unknown tools get the marker without a bash-specific hint")
	}
}

func TestRegistryAppliesTheCapToEveryTool(t *testing.T) {
	r := NewRegistry()
	r.Set("firehose", fakeTool{out: strings.Repeat("z\n", 100_000)})
	res := r.ExecuteTool("firehose", `{}`)
	if !res.Success || len(res.Output) > MaxToolOutputBytes+400 || !strings.Contains(res.Output, "output truncated") {
		t.Errorf("registry did not cap a plugin tool: success=%v len=%d", res.Success, len(res.Output))
	}
	// Errors carry their (possibly huge) partial output too.
	r.Set("failing", fakeTool{out: strings.Repeat("e\n", 100_000), err: "exit code 1"})
	res = r.ExecuteTool("failing", `{}`)
	if res.Success || res.Error != "exit code 1" || len(res.Output) > MaxToolOutputBytes+400 {
		t.Errorf("failed tool output not capped: %v %q len=%d", res.Success, res.Error, len(res.Output))
	}
}

type fakeTool struct {
	out string
	err string
}

func (f fakeTool) Execute(map[string]interface{}) ToolResult {
	return ToolResult{Success: f.err == "", Output: f.out, Error: f.err}
}

func TestBashToolLargeOutputIsCappedWithBothEnds(t *testing.T) {
	res := (&BashTool{}).Execute(map[string]interface{}{"command": "seq 1 100000"})
	if !res.Success {
		t.Fatalf("seq failed: %s", res.Error)
	}
	// The tool itself returns everything; the registry caps it. Both are
	// exercised: raw size here, capped size via the registry below.
	if len(res.Output) < 500_000 {
		t.Fatalf("expected ~590KB from seq, got %d", len(res.Output))
	}
	capped := NewRegistry().ExecuteTool("BashTool", `{"command":"seq 1 100000"}`)
	if len(capped.Output) > MaxToolOutputBytes+400 {
		t.Errorf("registry did not cap bash output: %d bytes", len(capped.Output))
	}
	if !strings.HasPrefix(capped.Output, "1\n2\n") || !strings.HasSuffix(capped.Output, "99999\n100000\n") {
		t.Errorf("head/tail not preserved: starts %q ends %q", capped.Output[:10], capped.Output[len(capped.Output)-14:])
	}
}

func TestFileReadToolPagesLargeFilesUnlessARangeIsGiven(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.txt")
	if err := os.WriteFile(path, []byte(numberedLines(5000)), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &FileReadTool{}

	// No range: first page plus a continuation hint naming the next line.
	res := tool.Execute(map[string]interface{}{"path": path})
	if !res.Success {
		t.Fatal(res.Error)
	}
	if !strings.HasPrefix(res.Output, "1: line 000001\n") || !strings.Contains(res.Output, "\n2000: line 002000\n") || strings.Contains(res.Output, "\n2001: ") {
		t.Errorf("first page wrong: starts %q", res.Output[:40])
	}
	if !strings.Contains(res.Output, "file has 5000 lines; showing 1-2000. Pass start_line=2001") {
		t.Errorf("continuation hint missing or wrong: %q", res.Output[len(res.Output)-200:])
	}

	// start_line only: a page from there.
	res = tool.Execute(map[string]interface{}{"path": path, "start_line": 2001})
	if !strings.HasPrefix(res.Output, "2001: ") || !strings.Contains(res.Output, "\n4000: ") || !strings.Contains(res.Output, "Pass start_line=4001") {
		t.Errorf("second page wrong")
	}

	// Explicit range is honoured exactly, even when large.
	res = tool.Execute(map[string]interface{}{"path": path, "start_line": 10, "end_line": 12})
	if res.Output != "10: line 000010\n11: line 000011\n12: line 000012\n" {
		t.Errorf("explicit range = %q", res.Output)
	}
	res = tool.Execute(map[string]interface{}{"path": path, "start_line": 1, "end_line": 3000})
	if strings.Count(res.Output, "\n") != 3000 || strings.Contains(res.Output, "[gocode:") {
		t.Errorf("explicit 3000-line range should be returned whole with no paging hint; got %d lines", strings.Count(res.Output, "\n"))
	}

	// A small file is unchanged: no hint, and the trailing newline is not
	// counted as an extra empty line.
	small := filepath.Join(t.TempDir(), "small.txt")
	if err := os.WriteFile(small, []byte("a\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res = tool.Execute(map[string]interface{}{"path": small})
	if res.Output != "1: a\n2: b\n" {
		t.Errorf("small file = %q", res.Output)
	}
}
