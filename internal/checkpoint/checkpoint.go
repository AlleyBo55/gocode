package checkpoint

import (
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Checkpoint represents a single git checkpoint.
type Checkpoint struct {
	ID        int
	Ref       string
	Message   string
	Timestamp time.Time
	CommitSHA string
}

// Manager creates and restores git checkpoints.
type Manager struct {
	sessionID string
	repoRoot  string
	refPrefix string
}

// NewManager creates a checkpoint manager for the given session.
func NewManager(sessionID, repoRoot string) *Manager {
	return &Manager{
		sessionID: sessionID,
		repoRoot:  repoRoot,
		refPrefix: "refs/gocode/checkpoints/" + sessionID + "/",
	}
}

// nextID returns the next checkpoint ID by scanning existing refs.
func (m *Manager) nextID() (int, error) {
	checkpoints, err := m.List()
	if err != nil {
		return 1, nil // no refs yet
	}
	if len(checkpoints) == 0 {
		return 1, nil
	}
	max := 0
	for _, cp := range checkpoints {
		if cp.ID > max {
			max = cp.ID
		}
	}
	return max + 1, nil
}

// gitCmd runs a git command in the repo root and returns its output.
func (m *Manager) gitCmd(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = m.repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// Create creates a checkpoint commit under the ref namespace.
// Stages only the specified files, creates a tree and commit object, then stores the ref.
func (m *Manager) Create(files []string, message string) (*Checkpoint, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no files specified for checkpoint")
	}

	// Verify we're in a git repo
	if _, err := m.gitCmd("rev-parse", "--git-dir"); err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}

	// Stage the specified files
	addArgs := append([]string{"add", "--"}, files...)
	if _, err := m.gitCmd(addArgs...); err != nil {
		return nil, fmt.Errorf("staging files: %w", err)
	}

	// Create a tree object from the current index
	treeSHA, err := m.gitCmd("write-tree")
	if err != nil {
		return nil, fmt.Errorf("write-tree: %w", err)
	}

	// Determine parent commit (previous checkpoint or none)
	id, err := m.nextID()
	if err != nil {
		return nil, err
	}

	var commitSHA string
	commitArgs := []string{"commit-tree", treeSHA, "-m", message}

	// If there's a previous checkpoint, use it as parent
	if id > 1 {
		prevRef := fmt.Sprintf("%s%d", m.refPrefix, id-1)
		prevSHA, err := m.gitCmd("rev-parse", prevRef)
		if err == nil && prevSHA != "" {
			commitArgs = append(commitArgs, "-p", prevSHA)
		}
	}

	commitSHA, err = m.gitCmd(commitArgs...)
	if err != nil {
		return nil, fmt.Errorf("commit-tree: %w", err)
	}

	// Store the ref
	ref := fmt.Sprintf("%s%d", m.refPrefix, id)
	if _, err := m.gitCmd("update-ref", ref, commitSHA); err != nil {
		return nil, fmt.Errorf("update-ref: %w", err)
	}

	return &Checkpoint{
		ID:        id,
		Ref:       ref,
		Message:   message,
		Timestamp: time.Now(),
		CommitSHA: commitSHA,
	}, nil
}

// List returns all checkpoints for the current session, ordered by ID.
func (m *Manager) List() ([]Checkpoint, error) {
	// Verify we're in a git repo
	if _, err := m.gitCmd("rev-parse", "--git-dir"); err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}

	out, err := m.gitCmd("for-each-ref", "--format=%(refname) %(objectname) %(creatordate:iso-strict) %(subject)", m.refPrefix)
	if err != nil {
		return nil, fmt.Errorf("listing refs: %w", err)
	}

	if out == "" {
		return nil, nil
	}

	var checkpoints []Checkpoint
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse: refname sha timestamp subject
		parts := strings.SplitN(line, " ", 4)
		if len(parts) < 3 {
			continue
		}

		refName := parts[0]
		sha := parts[1]
		tsStr := parts[2]
		msg := ""
		if len(parts) >= 4 {
			msg = parts[3]
		}

		// Extract ID from ref name
		idStr := strings.TrimPrefix(refName, m.refPrefix)
		id, err := strconv.Atoi(idStr)
		if err != nil {
			continue
		}

		ts, _ := time.Parse(time.RFC3339, tsStr)

		checkpoints = append(checkpoints, Checkpoint{
			ID:        id,
			Ref:       refName,
			Message:   msg,
			Timestamp: ts,
			CommitSHA: sha,
		})
	}

	sort.Slice(checkpoints, func(i, j int) bool {
		return checkpoints[i].ID < checkpoints[j].ID
	})

	return checkpoints, nil
}

// Restore rolls back the working tree to the state at checkpoint N.
// Removes refs for checkpoints after N.
func (m *Manager) Restore(id int) error {
	// Verify we're in a git repo
	if _, err := m.gitCmd("rev-parse", "--git-dir"); err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	ref := fmt.Sprintf("%s%d", m.refPrefix, id)

	// Verify the ref exists
	if _, err := m.gitCmd("rev-parse", ref); err != nil {
		return fmt.Errorf("checkpoint %d not found", id)
	}

	// Restore working tree to checkpoint state
	if _, err := m.gitCmd("checkout", ref, "--", "."); err != nil {
		return fmt.Errorf("restoring checkpoint %d: %w", id, err)
	}

	// Remove refs with ID > target
	checkpoints, err := m.List()
	if err != nil {
		return fmt.Errorf("listing checkpoints for cleanup: %w", err)
	}

	for _, cp := range checkpoints {
		if cp.ID > id {
			if _, err := m.gitCmd("update-ref", "-d", cp.Ref); err != nil {
				return fmt.Errorf("removing ref %s: %w", cp.Ref, err)
			}
		}
	}

	return nil
}

// Cleanup removes all checkpoint refs for the session.
func (m *Manager) Cleanup(squash bool) error {
	// Verify we're in a git repo
	if _, err := m.gitCmd("rev-parse", "--git-dir"); err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	checkpoints, err := m.List()
	if err != nil {
		return err
	}

	for _, cp := range checkpoints {
		if _, err := m.gitCmd("update-ref", "-d", cp.Ref); err != nil {
			return fmt.Errorf("removing ref %s: %w", cp.Ref, err)
		}
	}

	return nil
}
