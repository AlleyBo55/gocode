package repl

import (
	"bytes"
	"fmt"
	"testing"
)

func TestBannerRenders(t *testing.T) {
	var buf bytes.Buffer
	PrintBanner(&buf, BannerConfig{
		Version:  "v1.0.0",
		Model:    "claude-sonnet-4-6",
		MaxTurns: 30,
		Cwd:      "/Users/dev/myproject",
	})
	output := buf.String()
	if output == "" {
		t.Fatal("banner should not be empty")
	}
	fmt.Print(output)
}
