package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gaia/kernel"
)

// tasksMCPTools returns the MCP tools exposed by the tasks plugin.
func tasksMCPTools() []kernel.MCPTool {
	return []kernel.MCPTool{
		mcpTasksList(),
		mcpTasksGet(),
		mcpTasksAdd(),
		mcpTasksUpdate(),
		mcpTasksDone(),
		mcpTasksLogTime(),
		mcpTasksPrioritize(),
		mcpTasksDaily(),
		mcpTasksWeekly(),
		mcpTasksTimesheet(),
	}
}

func mcpTasksList() kernel.MCPTool {
	return kernel.MCPTool{
		Name:        "tasks_list",
		Description: "List tasks from the board. Returns the board view grouped by Eisenhower quadrant, or a flat list sorted by priority score.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"flat": map[string]interface{}{
					"type":        "boolean",
					"description": "Flat list sorted by priority score instead of board view",
				},
				"status": map[string]interface{}{
					"type":        "string",
					"description": "Filter by status: todo, in_progress, blocked_external, monitoring, pending_decision, backlog, done",
				},
				"projet": map[string]interface{}{
					"type":        "string",
					"description": "Filter by project name",
				},
			},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			store := NewStore()
			tasks, err := store.ListAll(ctx)
			if err != nil {
				return "", err
			}
			flat, _ := args["flat"].(bool)
			status, _ := args["status"].(string)
			project, _ := args["projet"].(string)
			filtered := filterTasks(tasks, status, project)
			view := BoardView(filtered, flat)
			if strings.TrimSpace(view) == "" {
				return "No active tasks found.", nil
			}
			return view, nil
		},
	}
}

func mcpTasksGet() kernel.MCPTool {
	return kernel.MCPTool{
		Name:        "tasks_get",
		Description: "Get a task by its T-number ID (e.g. T001).",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"id"},
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "Task ID, e.g. T001 or T047",
				},
			},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			id, _ := args["id"].(string)
			if id == "" {
				return "", fmt.Errorf("id is required")
			}
			store := NewStore()
			task, err := store.Get(ctx, id)
			if err != nil {
				return "", err
			}
			return FormatTaskContent(task), nil
		},
	}
}

func mcpTasksAdd() kernel.MCPTool {
	return kernel.MCPTool{
		Name:        "tasks_add",
		Description: "Create a new task. LLM inference is run automatically to populate effort, impact, eisenhower and priority score.",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"title"},
			"properties": map[string]interface{}{
				"title": map[string]interface{}{
					"type":        "string",
					"description": "Task title",
				},
				"projet": map[string]interface{}{
					"type":        "string",
					"description": "Project name (e.g. shared-devops)",
				},
				"deadline": map[string]interface{}{
					"type":        "string",
					"description": "Deadline in YYYY-MM-DD format",
				},
				"eisenhower": map[string]interface{}{
					"type":        "string",
					"description": "Eisenhower quadrant: Q1, Q2, Q3, or Q4",
					"enum":        []string{"Q1", "Q2", "Q3", "Q4"},
				},
			},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			title, _ := args["title"].(string)
			if title == "" {
				return "", fmt.Errorf("title is required")
			}
			project, _ := args["projet"].(string)
			deadline, _ := args["deadline"].(string)
			eisenhowerStr, _ := args["eisenhower"].(string)

			store := NewStore()
			maxID, err := store.NextID(ctx)
			if err != nil {
				return "", fmt.Errorf("NextID: %w", err)
			}

			task := Task{
				ID:         FormatTaskID(maxID + 1),
				Title:      title,
				Status:     StatusTodo,
				Project:    project,
				Deadline:   deadline,
				Eisenhower: Eisenhower(strings.ToUpper(eisenhowerStr)),
			}

			llm := newLLMClientFromConfig()
			meta, err := llm.InferTaskMeta(ctx, task)
			if err == nil {
				task.Effort = meta.Effort
				task.Impact = meta.Impact
				task.Category = meta.Category
				task.PriorityScore = meta.PriorityScore
				if task.Eisenhower == "" {
					task.Eisenhower = meta.Eisenhower
				}
			}

			created, err := store.Add(ctx, task)
			if err != nil {
				return "", fmt.Errorf("add task: %w", err)
			}

			var b strings.Builder
			fmt.Fprintf(&b, "Task %s créée (drawer: %s)\n", created.ID, created.DrawerID)
			if meta.PriorityScore > 0 {
				fmt.Fprintf(&b, "Score: %d | %s | effort: %s | impact: %s | category: %s\n",
					meta.PriorityScore, task.Eisenhower, task.Effort, task.Impact, task.Category)
			}
			return b.String(), nil
		},
	}
}

func mcpTasksUpdate() kernel.MCPTool {
	return kernel.MCPTool{
		Name:        "tasks_update",
		Description: "Update one or more fields on an existing task.",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"id"},
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "Task ID (e.g. T001)",
				},
				"title": map[string]interface{}{
					"type":        "string",
					"description": "New title",
				},
				"status": map[string]interface{}{
					"type":        "string",
					"description": "New status: todo, in_progress, blocked_external, monitoring, pending_decision, backlog, done",
				},
				"deadline": map[string]interface{}{
					"type":        "string",
					"description": "New deadline (YYYY-MM-DD)",
				},
				"priority": map[string]interface{}{
					"type":        "string",
					"description": "Priority label: HAUTE, MOYENNE, BASSE",
				},
			},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			id, _ := args["id"].(string)
			if id == "" {
				return "", fmt.Errorf("id is required")
			}
			store := NewStore()
			task, err := store.Get(ctx, id)
			if err != nil {
				return "", err
			}
			if title, _ := args["title"].(string); title != "" {
				task.Title = title
			}
			if status, _ := args["status"].(string); status != "" {
				task.Status = parseStatus(status)
			}
			if deadline, _ := args["deadline"].(string); deadline != "" {
				task.Deadline = deadline
			}
			if priority, _ := args["priority"].(string); priority != "" {
				task.Priority = strings.ToUpper(priority)
			}
			if err := store.Update(ctx, task); err != nil {
				return "", fmt.Errorf("update: %w", err)
			}
			return fmt.Sprintf("Task %s mise à jour.", task.ID), nil
		},
	}
}

func mcpTasksDone() kernel.MCPTool {
	return kernel.MCPTool{
		Name:        "tasks_done",
		Description: "Mark a task as done.",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"id"},
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "Task ID (e.g. T001)",
				},
			},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			id, _ := args["id"].(string)
			if id == "" {
				return "", fmt.Errorf("id is required")
			}
			store := NewStore()
			task, err := store.Get(ctx, id)
			if err != nil {
				return "", err
			}
			task.Status = StatusDone
			if err := store.Update(ctx, task); err != nil {
				return "", fmt.Errorf("done: %w", err)
			}
			return fmt.Sprintf("Task %s marquée comme done.", task.ID), nil
		},
	}
}

func mcpTasksLogTime() kernel.MCPTool {
	return kernel.MCPTool{
		Name:        "tasks_log_time",
		Description: "Log time spent on a task. Duration can be '1h30', '90min', '2h', '30m', or minutes as integer.",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"id", "duration"},
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "Task ID (e.g. T001)",
				},
				"duration": map[string]interface{}{
					"type":        "string",
					"description": "Duration: '1h30', '90min', '2h', '30m', or minutes as integer string",
				},
				"type": map[string]interface{}{
					"type":        "string",
					"description": "Log type: work or meeting (default: work)",
					"enum":        []string{"work", "meeting"},
				},
				"note": map[string]interface{}{
					"type":        "string",
					"description": "Optional note",
				},
				"date": map[string]interface{}{
					"type":        "string",
					"description": "Date YYYY-MM-DD (default: today)",
				},
			},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			id, _ := args["id"].(string)
			if id == "" {
				return "", fmt.Errorf("id is required")
			}
			durationStr, _ := args["duration"].(string)
			if durationStr == "" {
				return "", fmt.Errorf("duration is required")
			}
			mins, err := ParseDuration(durationStr)
			if err != nil {
				return "", fmt.Errorf("durée invalide: %w", err)
			}
			logType, _ := args["type"].(string)
			if logType == "" {
				logType = "work"
			}
			note, _ := args["note"].(string)
			date, _ := args["date"].(string)
			if date == "" {
				date = time.Now().Format("2006-01-02")
			}
			store := NewStore()
			if err := store.AddTimeLog(ctx, id, TimeLog{
				Date:            date,
				DurationMinutes: mins,
				Type:            logType,
				Source:          "manual",
				Note:            note,
			}); err != nil {
				return "", err
			}
			return fmt.Sprintf("Loggué %s sur %s (%s)", FormatMinutes(mins), id, date), nil
		},
	}
}

func mcpTasksPrioritize() kernel.MCPTool {
	return kernel.MCPTool{
		Name:        "tasks_prioritize",
		Description: "Re-compute priority scores for all active tasks using the LLM. Returns the updated ranking and a narrative.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Handler: func(ctx context.Context, _ map[string]interface{}) (string, error) {
			store := NewStore()
			tasks, err := store.ListAll(ctx)
			if err != nil {
				return "", err
			}
			active := make([]Task, 0, len(tasks))
			for _, t := range tasks {
				if t.IsActive() {
					active = append(active, t)
				}
			}
			if len(active) == 0 {
				return "Aucune task active.", nil
			}

			llm := newLLMClientFromConfig()
			today := time.Now().Format("2006-01-02")
			result, err := llm.Prioritize(ctx, active, today)
			if err != nil {
				return "", fmt.Errorf("LLM: %w", err)
			}

			byID := map[string]PrioritizedTask{}
			for _, pt := range result.Tasks {
				byID[pt.ID] = pt
			}
			for _, task := range active {
				pt, ok := byID[task.ID]
				if !ok {
					continue
				}
				task.PriorityScore = pt.PriorityScore
				task.Eisenhower = pt.Eisenhower
				task.Effort = pt.Effort
				task.Impact = pt.Impact
				task.Category = pt.Category
				if err := store.Update(ctx, task); err != nil {
					// non-fatal
					continue
				}
			}

			var b strings.Builder
			if result.Narrative != "" {
				b.WriteString(result.Narrative)
				b.WriteString("\n\n")
			}
			b.WriteString(BoardView(active, false))
			return b.String(), nil
		},
	}
}

func mcpTasksDaily() kernel.MCPTool {
	return kernel.MCPTool{
		Name:        "tasks_daily",
		Description: "Generate a daily standup report (hier / aujourd'hui / blocages).",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Handler: func(ctx context.Context, _ map[string]interface{}) (string, error) {
			store := NewStore()
			tasks, err := store.ListAll(ctx)
			if err != nil {
				return "", err
			}
			today := time.Now().Format("2006-01-02")
			yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
			return DailyReport(tasks, today, yesterday, ""), nil
		},
	}
}

func mcpTasksWeekly() kernel.MCPTool {
	return kernel.MCPTool{
		Name:        "tasks_weekly",
		Description: "Generate a weekly progress report grouped by objective.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"from": map[string]interface{}{
					"type":        "string",
					"description": "Start date YYYY-MM-DD (default: Monday of current week)",
				},
				"to": map[string]interface{}{
					"type":        "string",
					"description": "End date YYYY-MM-DD (default: Friday of current week)",
				},
			},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			from, _ := args["from"].(string)
			to, _ := args["to"].(string)
			if from == "" || to == "" {
				now := time.Now()
				weekday := int(now.Weekday())
				if weekday == 0 {
					weekday = 7
				}
				monday := now.AddDate(0, 0, -(weekday - 1))
				from = monday.Format("2006-01-02")
				to = monday.AddDate(0, 0, 4).Format("2006-01-02")
			}
			store := NewStore()
			tasks, err := store.ListAll(ctx)
			if err != nil {
				return "", err
			}
			return WeeklyReport(tasks, from, to), nil
		},
	}
}

func mcpTasksTimesheet() kernel.MCPTool {
	return kernel.MCPTool{
		Name:        "tasks_timesheet",
		Description: "Generate a timesheet (Mon-Fri) split by dev/management for the given week.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"week": map[string]interface{}{
					"type":        "string",
					"description": "Monday of the week YYYY-MM-DD (default: current week)",
				},
			},
		},
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			weekStr, _ := args["week"].(string)
			var monday time.Time
			if weekStr != "" {
				d, err := time.Parse("2006-01-02", weekStr)
				if err != nil {
					return "", fmt.Errorf("date invalide: %w", err)
				}
				monday = d
			} else {
				now := time.Now()
				weekday := int(now.Weekday())
				if weekday == 0 {
					weekday = 7
				}
				monday = now.AddDate(0, 0, -(weekday - 1))
			}
			from := monday.Format("2006-01-02")
			to := monday.AddDate(0, 0, 4).Format("2006-01-02")

			store := NewStore()
			tasks, err := store.ListAll(ctx)
			if err != nil {
				return "", err
			}
			return TimesheetReport(tasks, from, to), nil
		},
	}
}
