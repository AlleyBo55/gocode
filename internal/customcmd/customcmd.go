// Package customcmd discovers and loads user-defined slash commands from markdown
// files. Commands are loaded from two directories: project-level (.gocode/commands/)
// and user-level (~/.gocode/commands/). Project commands override user commands
// when names collide.
//
// Each .md file becomes a slash command whose name is the filename without the
// extension. An optional YAML frontmatter block (delimited by ---) can specify
// a description and positional argument definitions.
package customcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Command represents a loaded custom slash command.
type Command struct {
	Name        string   // derived from filename (without .md)
	Description string   // from YAML frontmatter
	Args        []ArgDef // from frontmatter "args" field
	Body        string   // markdown content (prompt template)
	Source      string   // "project" or "user"
	Path        string   // filesystem path
}

// ArgDef defines a positional argument placeholder.
type ArgDef struct {
	Name     string
	Required bool
}

// Loader discovers and loads custom commands from project and user directories.
type Loader struct {
	projectDir string // .gocode/commands/
	userDir    string // ~/.gocode/commands/
}

// NewLoader creates a Loader with default directories.
// projectDir is relative to the current working directory; userDir is under $HOME.
func NewLoader() *Loader {
	home, _ := os.UserHomeDir()
	return &Loader{
		projectDir: filepath.Join(".gocode", "commands"),
		userDir:    filepath.Join(home, ".gocode", "commands"),
	}
}

// NewLoaderWithDirs creates a Loader with explicit directories (useful for testing).
func NewLoaderWithDirs(projectDir, userDir string) *Loader {
	return &Loader{
		projectDir: projectDir,
		userDir:    userDir,
	}
}

// LoadAll loads commands from both directories. Project commands override user
// commands when names collide. Returns all loaded commands and any error
// encountered while scanning directories.
func (l *Loader) LoadAll() ([]Command, error) {
	// Load user-level commands first.
	userCmds, err := loadDir(l.userDir, "user")
	if err != nil {
		return nil, fmt.Errorf("loading user commands: %w", err)
	}

	// Load project-level commands.
	projectCmds, err := loadDir(l.projectDir, "project")
	if err != nil {
		return nil, fmt.Errorf("loading project commands: %w", err)
	}

	// Build result: start with user commands, then let project commands override.
	byName := make(map[string]int) // name -> index in result
	var result []Command
	for _, cmd := range userCmds {
		byName[cmd.Name] = len(result)
		result = append(result, cmd)
	}
	for _, cmd := range projectCmds {
		if idx, exists := byName[cmd.Name]; exists {
			result[idx] = cmd // project overrides user
		} else {
			byName[cmd.Name] = len(result)
			result = append(result, cmd)
		}
	}

	return result, nil
}

// Resolve finds a command by name and substitutes $1, $2, ..., $N with args.
// Returns the expanded prompt text or an error if required args are missing.
func (l *Loader) Resolve(name string, args []string) (string, error) {
	cmds, err := l.LoadAll()
	if err != nil {
		return "", err
	}

	var cmd *Command
	for i := range cmds {
		if cmds[i].Name == name {
			cmd = &cmds[i]
			break
		}
	}
	if cmd == nil {
		return "", fmt.Errorf("unknown command: %s", name)
	}

	// Validate required args.
	for i, arg := range cmd.Args {
		if arg.Required && i >= len(args) {
			return "", fmt.Errorf("missing required argument: %s (position $%d)", arg.Name, i+1)
		}
	}

	// Substitute $1, $2, ... placeholders.
	body := cmd.Body
	for i, val := range args {
		placeholder := "$" + strconv.Itoa(i+1)
		body = strings.ReplaceAll(body, placeholder, val)
	}

	return body, nil
}

// loadDir scans a directory for .md files and parses each into a Command.
// Returns nil slice (not error) if the directory does not exist.
func loadDir(dir, source string) ([]Command, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var cmds []Command
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue // skip unreadable files
		}

		name := strings.TrimSuffix(entry.Name(), ".md")
		cmd := parseCommand(string(data), name, source, path)
		cmds = append(cmds, cmd)
	}

	return cmds, nil
}

// parseCommand parses a markdown file's content into a Command.
// If the content starts with "---\n", it extracts YAML frontmatter for
// description and args. Otherwise the entire content becomes the body.
func parseCommand(content, name, source, path string) Command {
	cmd := Command{
		Name:   name,
		Source: source,
		Path:   path,
	}

	// Check for YAML frontmatter delimited by "---".
	if strings.HasPrefix(content, "---\n") || strings.HasPrefix(content, "---\r\n") {
		// Find the closing "---".
		rest := content[4:] // skip opening "---\n"
		if idx := strings.Index(rest, "\n---"); idx >= 0 {
			frontmatter := rest[:idx]
			// Body starts after the closing "---" line.
			afterClose := rest[idx+4:] // skip "\n---"
			// Trim the leading newline from body.
			if strings.HasPrefix(afterClose, "\n") {
				afterClose = afterClose[1:]
			} else if strings.HasPrefix(afterClose, "\r\n") {
				afterClose = afterClose[2:]
			}
			cmd.Body = afterClose
			parseFrontmatter(&cmd, frontmatter)
			return cmd
		}
	}

	// No frontmatter — entire content is the body.
	cmd.Body = content
	return cmd
}

// parseFrontmatter extracts description and args from a simple YAML-like
// frontmatter block. This is a minimal parser that handles the expected format
// without pulling in a full YAML library.
//
// Expected format:
//
//	description: "Run my custom workflow"
//	args:
//	  - name: target
//	    required: true
//	  - name: mode
//	    required: false
func parseFrontmatter(cmd *Command, fm string) {
	lines := strings.Split(fm, "\n")
	var inArgs bool
	var currentArg *ArgDef

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Detect top-level keys.
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "-") {
			inArgs = false
			if strings.HasPrefix(trimmed, "description:") {
				val := strings.TrimPrefix(trimmed, "description:")
				cmd.Description = unquote(strings.TrimSpace(val))
			} else if trimmed == "args:" {
				inArgs = true
			}
			continue
		}

		if !inArgs {
			continue
		}

		// Inside args list.
		if strings.HasPrefix(trimmed, "- name:") || strings.HasPrefix(trimmed, "-name:") {
			// Flush previous arg.
			if currentArg != nil {
				cmd.Args = append(cmd.Args, *currentArg)
			}
			val := trimmed
			val = strings.TrimPrefix(val, "- name:")
			val = strings.TrimPrefix(val, "-name:")
			currentArg = &ArgDef{
				Name: unquote(strings.TrimSpace(val)),
			}
		} else if currentArg != nil && strings.HasPrefix(trimmed, "required:") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "required:"))
			currentArg.Required = val == "true"
		}
	}

	// Flush last arg.
	if currentArg != nil {
		cmd.Args = append(cmd.Args, *currentArg)
	}
}

// unquote removes surrounding double or single quotes from a string.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
