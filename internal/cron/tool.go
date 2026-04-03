package cron

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/AlleyBo55/gocode/internal/models"
	"github.com/AlleyBo55/gocode/internal/toolimpl"
)

// cronEntry is the JSON-serializable form of a scheduled cron task.
type cronEntry struct {
	ID          string `json:"id"`
	Expression  string `json:"expression"`
	Description string `json:"description"`
	Recurring   bool   `json:"recurring"`
	CreatedAt   string `json:"created_at"`
}

// ScheduleCronTool implements toolimpl.ToolExecutor for create/delete/list cron jobs.
type ScheduleCronTool struct {
	Scheduler *Scheduler
	DataDir   string // path to .gocode directory for persisting cron.json
}

// ToolDef returns the tool definition for schedule_cron.
func (t *ScheduleCronTool) ToolDef() models.ToolDefinition {
	return models.ToolDefinition{
		Name:        "schedule_cron",
		Description: "Create, delete, or list scheduled cron jobs. Supports standard 5-field cron expressions.",
		InputSchema: models.InputSchema{
			Type: "object",
			Properties: map[string]models.SchemaProperty{
				"action": {
					Type:        "string",
					Description: `Action to perform: "create", "delete", or "list"`,
				},
				"expression": {
					Type:        "string",
					Description: "Cron expression (5-field: minute hour day-of-month month day-of-week). Required for create.",
				},
				"description": {
					Type:        "string",
					Description: "Human-readable description of the scheduled task. Required for create.",
				},
				"id": {
					Type:        "string",
					Description: "ID of the cron job to delete. Required for delete.",
				},
			},
			Required: []string{"action"},
		},
	}
}

// Execute dispatches to create, delete, or list based on the action param.
func (t *ScheduleCronTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	action, _ := params["action"].(string)
	switch action {
	case "create":
		return t.create(params)
	case "delete":
		return t.delete(params)
	case "list":
		return t.list()
	default:
		return toolimpl.ToolResult{Success: false, Error: fmt.Sprintf("unknown action %q: must be create, delete, or list", action)}
	}
}

func (t *ScheduleCronTool) create(params map[string]interface{}) toolimpl.ToolResult {
	expr, _ := params["expression"].(string)
	if expr == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: expression"}
	}
	desc, _ := params["description"].(string)
	if desc == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: description"}
	}

	id, err := t.Scheduler.Create(expr, desc, true)
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: fmt.Sprintf("failed to create cron job: %v", err)}
	}

	if err := t.persist(); err != nil {
		return toolimpl.ToolResult{Success: false, Error: fmt.Sprintf("created cron job %s but failed to persist: %v", id, err)}
	}

	fields, _ := Parse(expr)
	nextRun := NextRun(fields, time.Now())

	data, _ := json.Marshal(map[string]string{
		"id":          id,
		"expression":  expr,
		"description": desc,
		"human":       ToHuman(expr),
		"next_run":    nextRun.Format(time.RFC3339),
	})
	return toolimpl.ToolResult{Success: true, Output: string(data)}
}

func (t *ScheduleCronTool) delete(params map[string]interface{}) toolimpl.ToolResult {
	id, _ := params["id"].(string)
	if id == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: id"}
	}

	if err := t.Scheduler.Delete(id); err != nil {
		return toolimpl.ToolResult{Success: false, Error: fmt.Sprintf("failed to delete cron job: %v", err)}
	}

	if err := t.persist(); err != nil {
		return toolimpl.ToolResult{Success: false, Error: fmt.Sprintf("deleted cron job %s but failed to persist: %v", id, err)}
	}

	data, _ := json.Marshal(map[string]string{"id": id, "status": "deleted"})
	return toolimpl.ToolResult{Success: true, Output: string(data)}
}

func (t *ScheduleCronTool) list() toolimpl.ToolResult {
	tasks := t.Scheduler.List()

	// Sort by ID for stable output.
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })

	items := make([]map[string]string, len(tasks))
	for i, task := range tasks {
		items[i] = map[string]string{
			"id":          task.ID,
			"expression":  task.Cron,
			"description": task.Prompt,
			"human":       ToHuman(task.Cron),
			"next_run":    task.NextRun.Format(time.RFC3339),
			"created_at":  task.CreatedAt.Format(time.RFC3339),
		}
	}

	data, _ := json.Marshal(items)
	return toolimpl.ToolResult{Success: true, Output: string(data)}
}

// persist writes the current scheduler state to .gocode/cron.json.
func (t *ScheduleCronTool) persist() error {
	tasks := t.Scheduler.List()
	entries := make([]cronEntry, len(tasks))
	for i, task := range tasks {
		entries[i] = cronEntry{
			ID:          task.ID,
			Expression:  task.Cron,
			Description: task.Prompt,
			Recurring:   task.Recurring,
			CreatedAt:   task.CreatedAt.Format(time.RFC3339),
		}
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling cron entries: %w", err)
	}

	if err := os.MkdirAll(t.DataDir, 0o755); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}

	path := filepath.Join(t.DataDir, "cron.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing cron.json: %w", err)
	}
	return nil
}

// RegisterCronTool registers the schedule_cron tool in the given registry.
func RegisterCronTool(registry *toolimpl.Registry, scheduler *Scheduler, dataDir string) {
	registry.Set("schedule_cron", &ScheduleCronTool{Scheduler: scheduler, DataDir: dataDir})
}
