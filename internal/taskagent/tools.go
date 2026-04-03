package taskagent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AlleyBo55/gocode/internal/toolimpl"
)

// TaskCreateTool spawns a new background task.
type TaskCreateTool struct {
	mgr *TaskManager
}

func (t *TaskCreateTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	desc, _ := params["description"].(string)
	if desc == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: description"}
	}
	task, err := t.mgr.Create(desc, func(ctx context.Context, out *OutputBuffer) error {
		// Default no-op work; real agent work would be injected at a higher level.
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: err.Error()}
	}
	data, _ := json.Marshal(map[string]string{
		"id":          task.ID,
		"description": task.Description,
		"state":       string(task.State),
	})
	return toolimpl.ToolResult{Success: true, Output: string(data)}
}

// TaskGetTool retrieves a task by ID.
type TaskGetTool struct {
	mgr *TaskManager
}

func (t *TaskGetTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	id, _ := params["id"].(string)
	if id == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: id"}
	}
	task, err := t.mgr.Get(id)
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: err.Error()}
	}
	data, _ := json.Marshal(map[string]interface{}{
		"id":          task.ID,
		"description": task.Description,
		"state":       string(task.State),
		"created_at":  task.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
	return toolimpl.ToolResult{Success: true, Output: string(data)}
}

// TaskListTool lists all tasks.
type TaskListTool struct {
	mgr *TaskManager
}

func (t *TaskListTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	tasks := t.mgr.List()
	items := make([]map[string]string, len(tasks))
	for i, task := range tasks {
		items[i] = map[string]string{
			"id":          task.ID,
			"description": task.Description,
			"state":       string(task.State),
		}
	}
	data, _ := json.Marshal(items)
	return toolimpl.ToolResult{Success: true, Output: string(data)}
}

// TaskOutputTool returns buffered output for a task.
type TaskOutputTool struct {
	mgr *TaskManager
}

func (t *TaskOutputTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	id, _ := params["id"].(string)
	if id == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: id"}
	}
	lines, err := t.mgr.Output(id)
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: err.Error()}
	}
	data, _ := json.Marshal(lines)
	return toolimpl.ToolResult{Success: true, Output: string(data)}
}

// TaskStopTool cancels a running task.
type TaskStopTool struct {
	mgr *TaskManager
}

func (t *TaskStopTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	id, _ := params["id"].(string)
	if id == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: id"}
	}
	if err := t.mgr.Stop(id); err != nil {
		return toolimpl.ToolResult{Success: false, Error: err.Error()}
	}
	return toolimpl.ToolResult{Success: true, Output: fmt.Sprintf("task %s stopped", id)}
}

// TaskUpdateTool modifies a task's description.
type TaskUpdateTool struct {
	mgr *TaskManager
}

func (t *TaskUpdateTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	id, _ := params["id"].(string)
	if id == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: id"}
	}
	desc, _ := params["description"].(string)
	if desc == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: description"}
	}
	if err := t.mgr.Update(id, desc); err != nil {
		return toolimpl.ToolResult{Success: false, Error: err.Error()}
	}
	return toolimpl.ToolResult{Success: true, Output: fmt.Sprintf("task %s updated", id)}
}

// RegisterTaskTools registers all six task tools in the given registry.
func RegisterTaskTools(registry *toolimpl.Registry, mgr *TaskManager) {
	registry.Set("taskcreatetool", &TaskCreateTool{mgr: mgr})
	registry.Set("taskgettool", &TaskGetTool{mgr: mgr})
	registry.Set("tasklisttool", &TaskListTool{mgr: mgr})
	registry.Set("taskoutputtool", &TaskOutputTool{mgr: mgr})
	registry.Set("taskstoptool", &TaskStopTool{mgr: mgr})
	registry.Set("taskupdatetool", &TaskUpdateTool{mgr: mgr})
}
