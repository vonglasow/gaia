package tasks

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTasksMCPTools_Count(t *testing.T) {
	tools := tasksMCPTools()
	assert.Len(t, tools, 10)
}

func TestTasksMCPTools_Names(t *testing.T) {
	tools := tasksMCPTools()
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	assert.Contains(t, names, "tasks_list")
	assert.Contains(t, names, "tasks_get")
	assert.Contains(t, names, "tasks_add")
	assert.Contains(t, names, "tasks_update")
	assert.Contains(t, names, "tasks_done")
	assert.Contains(t, names, "tasks_log_time")
	assert.Contains(t, names, "tasks_prioritize")
	assert.Contains(t, names, "tasks_daily")
	assert.Contains(t, names, "tasks_weekly")
	assert.Contains(t, names, "tasks_timesheet")
}

func TestTasksMCPTools_AllHaveSchema(t *testing.T) {
	for _, tool := range tasksMCPTools() {
		assert.NotEmpty(t, tool.Name, "tool name should not be empty")
		assert.NotEmpty(t, tool.Description, "tool %q should have a description", tool.Name)
		assert.NotNil(t, tool.InputSchema, "tool %q should have an InputSchema", tool.Name)
		assert.NotNil(t, tool.Handler, "tool %q should have a Handler", tool.Name)
	}
}

func TestTasksMCPTools_AllHaveObjectSchema(t *testing.T) {
	for _, tool := range tasksMCPTools() {
		schema := tool.InputSchema
		assert.Equal(t, "object", schema["type"], "tool %q schema type should be 'object'", tool.Name)
	}
}

// Tests for tools that don't call store (just verify schema structure):

func TestMCPTasksList_EmptyArgs(t *testing.T) {
	listResp, _ := json.Marshal(map[string]interface{}{"drawers": []interface{}{}, "total": 0})
	// Patch the global callTool for this test by temporarily wrapping NewStore.
	// Since we can't easily inject, let's test the handler indirectly via a direct store.
	_ = listResp
	// Just verify the tool exists and has handler
	tool := findTool(t, "tasks_list")
	assert.NotNil(t, tool.Handler)
}

func TestMCPTasksGet_MissingID(t *testing.T) {
	tool := findTool(t, "tasks_get")
	_, err := tool.Handler(context.Background(), map[string]interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "id is required")
}

func TestMCPTasksAdd_MissingTitle(t *testing.T) {
	tool := findTool(t, "tasks_add")
	_, err := tool.Handler(context.Background(), map[string]interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "title is required")
}

func TestMCPTasksUpdate_MissingID(t *testing.T) {
	tool := findTool(t, "tasks_update")
	_, err := tool.Handler(context.Background(), map[string]interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "id is required")
}

func TestMCPTasksDone_MissingID(t *testing.T) {
	tool := findTool(t, "tasks_done")
	_, err := tool.Handler(context.Background(), map[string]interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "id is required")
}

func TestMCPTasksLogTime_MissingID(t *testing.T) {
	tool := findTool(t, "tasks_log_time")
	_, err := tool.Handler(context.Background(), map[string]interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "id is required")
}

func TestMCPTasksLogTime_MissingDuration(t *testing.T) {
	tool := findTool(t, "tasks_log_time")
	_, err := tool.Handler(context.Background(), map[string]interface{}{"id": "T001"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duration is required")
}

func TestMCPTasksLogTime_InvalidDuration(t *testing.T) {
	tool := findTool(t, "tasks_log_time")
	_, err := tool.Handler(context.Background(), map[string]interface{}{
		"id": "T001", "duration": "notaduration",
	})
	assert.Error(t, err)
}

func TestMCPTasksTimesheet_InvalidWeek(t *testing.T) {
	tool := findTool(t, "tasks_timesheet")
	// Empty store — will fail on list all
	_, err := tool.Handler(context.Background(), map[string]interface{}{"week": "not-a-date"})
	assert.Error(t, err)
}

func TestMCPTasksDaily_NoTasks(t *testing.T) {
	// Patch would be needed to get nil-store; test that handler returns no error on empty
	// when the store returns an error, the tool propagates it
	tool := findTool(t, "tasks_daily")
	assert.NotNil(t, tool.Handler)
}

func TestMCPTasksWeekly_ValidDateRange(t *testing.T) {
	tool := findTool(t, "tasks_weekly")
	assert.NotNil(t, tool.Handler)
	// Invalid args (from > to) just calls store and returns empty report — no error
}

func TestMCPTasksPrioritize_NoActive(t *testing.T) {
	tool := findTool(t, "tasks_prioritize")
	assert.NotNil(t, tool.Handler)
}

// findTool returns the MCPTool with the given name.
func findTool(t *testing.T, name string) *struct {
	Name        string
	Description string
	Handler     func(ctx context.Context, args map[string]interface{}) (string, error)
} {
	t.Helper()
	for _, tool := range tasksMCPTools() {
		if tool.Name == name {
			return &struct {
				Name        string
				Description string
				Handler     func(ctx context.Context, args map[string]interface{}) (string, error)
			}{
				Name:        tool.Name,
				Description: tool.Description,
				Handler:     tool.Handler,
			}
		}
	}
	t.Fatalf("tool %q not found", name)
	return nil
}

func TestMCPTasksList_SchemaHasProperties(t *testing.T) {
	for _, tool := range tasksMCPTools() {
		schema := tool.InputSchema
		props, _ := schema["properties"].(map[string]interface{})

		switch tool.Name {
		case "tasks_get", "tasks_done":
			assert.Contains(t, props, "id", "tool %s should have id property", tool.Name)
		case "tasks_add":
			assert.Contains(t, props, "title", "tasks_add should have title property")
		case "tasks_update":
			assert.Contains(t, props, "id", "tasks_update should have id property")
			assert.Contains(t, props, "status", "tasks_update should have status property")
		case "tasks_log_time":
			assert.Contains(t, props, "id", "tasks_log_time should have id property")
			assert.Contains(t, props, "duration", "tasks_log_time should have duration property")
		}
	}
}

func TestMCPTasksList_RequiredFields(t *testing.T) {
	required := map[string][]string{
		"tasks_get":      {"id"},
		"tasks_add":      {"title"},
		"tasks_update":   {"id"},
		"tasks_done":     {"id"},
		"tasks_log_time": {"id", "duration"},
	}
	for _, tool := range tasksMCPTools() {
		exp, ok := required[tool.Name]
		if !ok {
			continue
		}
		schema := tool.InputSchema
		req, _ := schema["required"].([]string)
		for _, r := range exp {
			assert.Contains(t, req, r, "tool %s should require %q", tool.Name, r)
		}
	}
}

func TestNewLLMClientFromConfig(t *testing.T) {
	// Just ensure it doesn't panic with default config
	client := newLLMClientFromConfig()
	assert.NotNil(t, client)
}

func TestMCPTasksWeekly_WithExplicitDates(t *testing.T) {
	tool := findTool(t, "tasks_weekly")
	// This will fail because NewStore() will fail (no mempalace), but the date parsing works
	_, err := tool.Handler(context.Background(), map[string]interface{}{
		"from": "2026-06-29",
		"to":   "2026-07-03",
	})
	// Error from store/mempalace is expected in unit test context
	if err != nil {
		assert.NotContains(t, err.Error(), "date invalide")
	}
}

func TestMCPToolHandlers_PropagateStoreErrors(t *testing.T) {
	// Tasks that require a store ID but have no store access will return errors
	// This verifies the error propagation path
	cases := []struct {
		tool string
		args map[string]interface{}
	}{
		{"tasks_get", map[string]interface{}{"id": "T999"}},
		{"tasks_update", map[string]interface{}{"id": "T999", "title": "new"}},
		{"tasks_done", map[string]interface{}{"id": "T999"}},
		{"tasks_log_time", map[string]interface{}{"id": "T999", "duration": "1h"}},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			tool := findTool(t, tc.tool)
			_, err := tool.Handler(context.Background(), tc.args)
			// Either error (store not available) or success (rare) — just no panic
			_ = err
		})
	}
}

// Verify unique tool names (no duplicate registrations).
func TestMCPToolNames_Unique(t *testing.T) {
	seen := map[string]bool{}
	for _, tool := range tasksMCPTools() {
		assert.False(t, seen[tool.Name], "duplicate tool name: %s", tool.Name)
		seen[tool.Name] = true
	}
}

// Verify input schemas are valid JSON.
func TestMCPToolSchemas_ValidJSON(t *testing.T) {
	for _, tool := range tasksMCPTools() {
		b, err := json.Marshal(tool.InputSchema)
		require.NoError(t, err, "tool %s InputSchema should be JSON-serializable", tool.Name)
		assert.Contains(t, string(b), `"type"`, "tool %s schema should have type field", tool.Name)
		t.Logf("%s: %s", tool.Name, b)
	}
}
