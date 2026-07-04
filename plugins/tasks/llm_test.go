package tasks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeOllamaServer(t *testing.T, responseText string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"message": map[string]string{"role": "assistant", "content": responseText},
			"done":    true,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLLMClient_InferTaskMeta(t *testing.T) {
	payload := `{"effort":"medium","impact":"high","category":"dev","eisenhower":"Q2","priority_score":80}`
	srv := makeOllamaServer(t, payload)

	client := newLLMClientFromURL(srv.URL, 5*time.Second)

	meta, err := client.InferTaskMeta(context.Background(), Task{
		ID:    "T048",
		Title: "New task to categorize",
		Description: "Description here",
	})
	require.NoError(t, err)
	assert.Equal(t, EffortMedium, meta.Effort)
	assert.Equal(t, ImpactHigh, meta.Impact)
	assert.Equal(t, CategoryDev, meta.Category)
	assert.Equal(t, EisenhowerQ2, meta.Eisenhower)
	assert.Equal(t, 80, meta.PriorityScore)
}

func TestLLMClient_Prioritize(t *testing.T) {
	payload := `{
		"tasks":[
			{"id":"T001","priority_score":90,"eisenhower":"Q1","effort":"small","impact":"high","category":"dev"},
			{"id":"T002","priority_score":40,"eisenhower":"Q4","effort":"large","impact":"low","category":"management"}
		],
		"narrative":"Focus T001 car bloquant."
	}`
	srv := makeOllamaServer(t, payload)
	client := newLLMClientFromURL(srv.URL, 5*time.Second)

	tasks := []Task{
		{ID: "T001", Title: "Urgent fix", Status: StatusInProgress},
		{ID: "T002", Title: "Nice to have", Status: StatusTodo},
	}
	result, err := client.Prioritize(context.Background(), tasks, "2026-07-03")
	require.NoError(t, err)
	require.Len(t, result.Tasks, 2)
	assert.Equal(t, "T001", result.Tasks[0].ID)
	assert.Equal(t, 90, result.Tasks[0].PriorityScore)
	assert.Equal(t, "Focus T001 car bloquant.", result.Narrative)
}

func TestLLMClient_Prioritize_InvalidJSON(t *testing.T) {
	srv := makeOllamaServer(t, "not valid json at all")
	client := newLLMClientFromURL(srv.URL, 5*time.Second)

	_, err := client.Prioritize(context.Background(), []Task{{ID: "T001", Title: "x"}}, "2026-07-03")
	assert.Error(t, err)
}

func TestLLMClient_InferSessions(t *testing.T) {
	payload := `{
		"entries":[
			{"task_id":"T041","date":"2026-07-03","duration_minutes":45,"confidence":0.9}
		]
	}`
	srv := makeOllamaServer(t, payload)
	client := newLLMClientFromURL(srv.URL, 5*time.Second)

	tasks := []Task{{ID: "T041", Title: "CI tests", Project: "shared-devops"}}
	sessions := []SessionSummary{{
		Date:            "2026-07-03",
		DurationMinutes: 45,
		ProjectPath:     "/Users/x/Documents/chapsvision",
		Messages:        []string{"Let me fix the CI tests for Renovate"},
	}}

	entries, err := client.InferSessions(context.Background(), tasks, sessions)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "T041", entries[0].TaskID)
	assert.Equal(t, 45, entries[0].DurationMinutes)
}

func TestLLMClient_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	client := newLLMClientFromURL(srv.URL, 5*time.Second)

	_, err := client.InferTaskMeta(context.Background(), Task{ID: "T001", Title: "x"})
	assert.Error(t, err)
}
