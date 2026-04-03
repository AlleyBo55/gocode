package checkpoint

import (
	"github.com/AlleyBo55/gocode/internal/toolimpl"
)

// CheckpointToolWrapper wraps a ToolExecutor and creates checkpoints on success.
type CheckpointToolWrapper struct {
	inner   toolimpl.ToolExecutor
	manager *Manager
}

// NewCheckpointToolWrapper creates a wrapper that checkpoints after successful tool execution.
func NewCheckpointToolWrapper(inner toolimpl.ToolExecutor, manager *Manager) *CheckpointToolWrapper {
	return &CheckpointToolWrapper{
		inner:   inner,
		manager: manager,
	}
}

// Execute runs the inner tool and creates a checkpoint if it succeeds.
func (w *CheckpointToolWrapper) Execute(params map[string]interface{}) toolimpl.ToolResult {
	result := w.inner.Execute(params)
	if !result.Success {
		return result
	}

	// Extract the file path from params for the checkpoint
	path, _ := params["path"].(string)
	if path == "" {
		path, _ = params["file_path"].(string)
	}
	if path == "" {
		return result
	}

	msg := "checkpoint after file write: " + path
	_, _ = w.manager.Create([]string{path}, msg)

	return result
}
