package repl

import "testing"

func TestParseSlashCommand_UltraPlan(t *testing.T) {
	tests := []struct {
		input string
		want  SlashCommand
	}{
		{"/ultraplan", CmdUltraPlan},
		{"/ultraplan refactor the auth module", CmdUltraPlan},
		{"/ULTRAPLAN", CmdUltraPlan},
		{"ultraplan", CmdNone}, // no slash = not a command
	}
	for _, tt := range tests {
		got := ParseSlashCommand(tt.input)
		if got != tt.want {
			t.Errorf("ParseSlashCommand(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
