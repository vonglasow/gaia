package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTestStore(responses map[string]json.RawMessage) *Store {
	return &Store{
		callTool: func(_ context.Context, tool string, args map[string]interface{}) (json.RawMessage, error) {
			key := tool
			if drawerID, ok := args["drawer_id"].(string); ok {
				key = tool + ":" + drawerID
			}
			if raw, ok := responses[key]; ok {
				return raw, nil
			}
			return nil, fmt.Errorf("unexpected tool call: %s %v", tool, args)
		},
	}
}

func TestStore_ListAll(t *testing.T) {
	drawerID := "drawer_tasks_board_abc"
	taskContent := FormatTaskContent(Task{
		ID: "T001", Title: "Test task", Status: StatusTodo, Project: "shared-devops",
	})

	listResp, _ := json.Marshal(map[string]interface{}{
		"drawers": []map[string]interface{}{
			{"drawer_id": drawerID, "content_preview": "## T001"},
		},
		"total": 1,
	})
	getResp, _ := json.Marshal(map[string]interface{}{
		"drawer_id": drawerID,
		"content":   taskContent,
	})

	store := makeTestStore(map[string]json.RawMessage{
		"mempalace_list_drawers":           listResp,
		"mempalace_get_drawer:" + drawerID: getResp,
	})

	tasks, err := store.ListAll(context.Background())
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "T001", tasks[0].ID)
	assert.Equal(t, "Test task", tasks[0].Title)
	assert.Equal(t, drawerID, tasks[0].DrawerID)
}

func TestStore_ListAll_SkipsNonTaskDrawers(t *testing.T) {
	listResp, _ := json.Marshal(map[string]interface{}{
		"drawers": []map[string]interface{}{
			{"drawer_id": "drawer_tasks_board_abc", "content_preview": "not a task"},
		},
		"total": 1,
	})
	getResp, _ := json.Marshal(map[string]interface{}{
		"drawer_id": "drawer_tasks_board_abc",
		"content":   "no heading here",
	})

	store := makeTestStore(map[string]json.RawMessage{
		"mempalace_list_drawers":                      listResp,
		"mempalace_get_drawer:drawer_tasks_board_abc": getResp,
	})

	tasks, err := store.ListAll(context.Background())
	require.NoError(t, err)
	assert.Len(t, tasks, 0)
}

func TestStore_Get_Found(t *testing.T) {
	drawerID := "drawer_tasks_board_xyz"
	taskContent := FormatTaskContent(Task{ID: "T047", Title: "Found it", Status: StatusInProgress})

	listResp, _ := json.Marshal(map[string]interface{}{
		"drawers": []map[string]interface{}{{"drawer_id": drawerID}},
		"total":   1,
	})
	getResp, _ := json.Marshal(map[string]interface{}{"content": taskContent})

	store := makeTestStore(map[string]json.RawMessage{
		"mempalace_list_drawers":           listResp,
		"mempalace_get_drawer:" + drawerID: getResp,
	})

	task, err := store.Get(context.Background(), "T047")
	require.NoError(t, err)
	assert.Equal(t, "T047", task.ID)
}

func TestStore_Get_NotFound(t *testing.T) {
	listResp, _ := json.Marshal(map[string]interface{}{"drawers": []interface{}{}, "total": 0})
	store := makeTestStore(map[string]json.RawMessage{"mempalace_list_drawers": listResp})

	_, err := store.Get(context.Background(), "T999")
	assert.Error(t, err)
}

func TestStore_Add(t *testing.T) {
	addResp, _ := json.Marshal(map[string]interface{}{
		"success":   true,
		"drawer_id": "drawer_tasks_board_new123",
	})
	store := makeTestStore(map[string]json.RawMessage{
		"mempalace_add_drawer": addResp,
	})

	task := Task{ID: "T048", Title: "New task", Status: StatusTodo}
	created, err := store.Add(context.Background(), task)
	require.NoError(t, err)
	assert.Equal(t, "T048", created.ID)
	assert.Equal(t, "drawer_tasks_board_new123", created.DrawerID)
}

func TestStore_Add_Error(t *testing.T) {
	store := &Store{
		callTool: func(_ context.Context, tool string, _ map[string]interface{}) (json.RawMessage, error) {
			return nil, fmt.Errorf("mempalace unavailable")
		},
	}
	_, err := store.Add(context.Background(), Task{ID: "T048", Title: "x", Status: StatusTodo})
	assert.Error(t, err)
}

func TestStore_Update(t *testing.T) {
	drawerID := "drawer_tasks_board_abc"
	existing := FormatTaskContent(Task{ID: "T001", Title: "Old", Status: StatusTodo})

	getResp, _ := json.Marshal(map[string]interface{}{"content": existing})
	updateResp, _ := json.Marshal(map[string]interface{}{"success": true})

	calls := map[string]int{}
	store := &Store{
		callTool: func(_ context.Context, tool string, args map[string]interface{}) (json.RawMessage, error) {
			calls[tool]++
			switch tool {
			case "mempalace_get_drawer":
				return getResp, nil
			case "mempalace_update_drawer":
				return updateResp, nil
			}
			return nil, fmt.Errorf("unexpected: %s", tool)
		},
	}

	err := store.Update(context.Background(), Task{
		ID: "T001", DrawerID: drawerID, Title: "Updated", Status: StatusInProgress,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, calls["mempalace_get_drawer"])
	assert.Equal(t, 1, calls["mempalace_update_drawer"])
}

func TestStore_Update_NoDrawerID(t *testing.T) {
	store := makeTestStore(map[string]json.RawMessage{})
	err := store.Update(context.Background(), Task{ID: "T001", Title: "x"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no drawer ID")
}

func TestStore_AddTimeLog(t *testing.T) {
	drawerID := "drawer_tasks_board_abc"
	existing := FormatTaskContent(Task{ID: "T001", Title: "T", Status: StatusInProgress, DrawerID: drawerID})

	listResp, _ := json.Marshal(map[string]interface{}{
		"drawers": []map[string]interface{}{{"drawer_id": drawerID}},
		"total":   1,
	})
	getResp, _ := json.Marshal(map[string]interface{}{"content": existing})
	updateResp, _ := json.Marshal(map[string]interface{}{"success": true})

	callCount := map[string]int{}
	store := &Store{
		callTool: func(_ context.Context, tool string, args map[string]interface{}) (json.RawMessage, error) {
			callCount[tool]++
			switch tool {
			case "mempalace_list_drawers":
				return listResp, nil
			case "mempalace_get_drawer":
				return getResp, nil
			case "mempalace_update_drawer":
				return updateResp, nil
			}
			return nil, fmt.Errorf("unexpected: %s", tool)
		},
	}

	err := store.AddTimeLog(context.Background(), "T001", TimeLog{
		Date: "2026-07-03", DurationMinutes: 60, Type: "work", Source: "manual",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, callCount["mempalace_update_drawer"])
}

func TestStore_NextID(t *testing.T) {
	drawerID := "drawer_tasks_board_abc"
	taskContent := FormatTaskContent(Task{ID: "T047", Title: "T", Status: StatusTodo})

	listResp, _ := json.Marshal(map[string]interface{}{
		"drawers": []map[string]interface{}{{"drawer_id": drawerID}},
		"total":   1,
	})
	getResp, _ := json.Marshal(map[string]interface{}{"content": taskContent})

	store := makeTestStore(map[string]json.RawMessage{
		"mempalace_list_drawers":           listResp,
		"mempalace_get_drawer:" + drawerID: getResp,
	})

	n, err := store.NextID(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 47, n)
}

func TestUnwrapMCPEnvelope(t *testing.T) {
	inner := `{"drawers":[],"total":0}`
	wrapped := fmt.Sprintf(`{"content":[{"type":"text","text":%q}]}`, inner)
	result := unwrapMCPContent(json.RawMessage(wrapped))
	assert.JSONEq(t, inner, string(result))
}

func TestUnwrapMCPEnvelope_PassThrough(t *testing.T) {
	direct := json.RawMessage(`{"drawers":[],"total":0}`)
	result := unwrapMCPContent(direct)
	assert.JSONEq(t, string(direct), string(result))
}
