package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gaia/plugins/mempalace"
)

const (
	Wing     = "tasks"
	RoomBoard = "board"
	RoomMeta  = "meta"
)

// CallToolFn is the Mempalace tool-call function signature.
type CallToolFn func(ctx context.Context, tool string, args map[string]interface{}) (json.RawMessage, error)

// Store manages tasks in Mempalace.
type Store struct {
	callTool CallToolFn
}

// NewStore creates a Store backed by the default Mempalace client.
func NewStore() *Store {
	return &Store{callTool: mempalace.CallTool}
}

// ListAll returns all tasks from the board room.
func (s *Store) ListAll(ctx context.Context) ([]Task, error) {
	raw, err := s.callTool(ctx, "mempalace_list_drawers", map[string]interface{}{
		"wing":  Wing,
		"room":  RoomBoard,
		"limit": 100,
	})
	if err != nil {
		return nil, fmt.Errorf("listing tasks: %w", err)
	}

	drawerIDs, err := extractDrawerIDs(unwrapMCPContent(raw))
	if err != nil {
		return nil, err
	}

	tasks := make([]Task, 0, len(drawerIDs))
	for _, id := range drawerIDs {
		content, err := s.getDrawerContent(ctx, id)
		if err != nil {
			continue
		}
		task, err := ParseTaskFromContent(id, content)
		if err != nil {
			continue
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// Get finds a task by T-number (case-insensitive).
func (s *Store) Get(ctx context.Context, taskID string) (Task, error) {
	all, err := s.ListAll(ctx)
	if err != nil {
		return Task{}, err
	}
	for _, t := range all {
		if strings.EqualFold(t.ID, taskID) {
			return t, nil
		}
	}
	return Task{}, fmt.Errorf("task %q not found", taskID)
}

// Add creates a new task drawer and returns the task with its DrawerID filled in.
func (s *Store) Add(ctx context.Context, task Task) (Task, error) {
	raw, err := s.callTool(ctx, "mempalace_add_drawer", map[string]interface{}{
		"wing":        Wing,
		"room":        RoomBoard,
		"content":     FormatTaskContent(task),
		"source_file": "gaia-tasks",
		"added_by":    "gaia",
	})
	if err != nil {
		return Task{}, fmt.Errorf("adding task: %w", err)
	}
	var result struct {
		DrawerID string `json:"drawer_id"`
	}
	if err := unmarshalMCP(raw, &result); err != nil {
		return Task{}, fmt.Errorf("parsing add response: %w", err)
	}
	task.DrawerID = result.DrawerID
	return task, nil
}

// Update replaces a task drawer's content.
func (s *Store) Update(ctx context.Context, task Task) error {
	if task.DrawerID == "" {
		return fmt.Errorf("task %q has no drawer ID", task.ID)
	}
	existing, err := s.getDrawerContent(ctx, task.DrawerID)
	if err != nil {
		return fmt.Errorf("reading drawer %s: %w", task.DrawerID, err)
	}
	_, err = s.callTool(ctx, "mempalace_update_drawer", map[string]interface{}{
		"drawer_id":  task.DrawerID,
		"old_string": existing,
		"new_string": FormatTaskContent(task),
	})
	return err
}

// AddTimeLog appends a time log entry to an existing task.
func (s *Store) AddTimeLog(ctx context.Context, taskID string, log TimeLog) error {
	task, err := s.Get(ctx, taskID)
	if err != nil {
		return err
	}
	task.TimeLogs = append(task.TimeLogs, log)
	return s.Update(ctx, task)
}

// NextID scans all tasks and returns the current maximum T-number.
func (s *Store) NextID(ctx context.Context) (int, error) {
	tasks, err := s.ListAll(ctx)
	if err != nil {
		return 0, err
	}
	maxN := 0
	for _, t := range tasks {
		if n := parseTaskNumber(t.ID); n > maxN {
			maxN = n
		}
	}
	return maxN, nil
}

func (s *Store) getDrawerContent(ctx context.Context, drawerID string) (string, error) {
	raw, err := s.callTool(ctx, "mempalace_get_drawer", map[string]interface{}{
		"drawer_id": drawerID,
	})
	if err != nil {
		return "", err
	}
	var result struct {
		Content string `json:"content"`
	}
	if err := unmarshalMCP(raw, &result); err != nil {
		return "", err
	}
	return result.Content, nil
}

// extractDrawerIDs parses a list_drawers response and returns drawer IDs.
func extractDrawerIDs(raw json.RawMessage) ([]string, error) {
	var envelope struct {
		Drawers []struct {
			DrawerID string `json:"drawer_id"`
		} `json:"drawers"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("parsing drawer list: %w", err)
	}
	ids := make([]string, 0, len(envelope.Drawers))
	for _, d := range envelope.Drawers {
		if d.DrawerID != "" {
			ids = append(ids, d.DrawerID)
		}
	}
	return ids, nil
}

// unwrapMCPContent strips the MCP tool-call envelope if present.
func unwrapMCPContent(raw json.RawMessage) json.RawMessage {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return raw
	}
	var envelope struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return raw
	}
	for _, item := range envelope.Content {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		var js interface{}
		if err := json.Unmarshal(json.RawMessage(text), &js); err == nil {
			return json.RawMessage(text)
		}
	}
	return raw
}

// unmarshalMCP unwraps the MCP envelope then unmarshals into v.
func unmarshalMCP(raw json.RawMessage, v interface{}) error {
	return json.Unmarshal(unwrapMCPContent(raw), v)
}
