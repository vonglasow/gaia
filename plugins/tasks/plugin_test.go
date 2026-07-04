package tasks

import (
	"testing"

	"gaia/kernel"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTasksPlugin_ID(t *testing.T) {
	p := NewTasksPlugin()
	assert.Equal(t, "tasks", p.ID())
}

func TestTasksPlugin_DefaultEnabled(t *testing.T) {
	p := NewTasksPlugin()
	assert.True(t, p.DefaultEnabled())
}

func TestTasksPlugin_DependsOn(t *testing.T) {
	p := NewTasksPlugin()
	assert.Equal(t, []string{"mempalace"}, p.DependsOn())
}

func TestTasksPlugin_ConfigSchema(t *testing.T) {
	p := NewTasksPlugin()
	schema := p.ConfigSchema()
	assert.Contains(t, schema, "tasks.ollama_host")
	assert.Contains(t, schema, "tasks.ollama_port")
	assert.Contains(t, schema, "tasks.model")
}

func TestTasksPlugin_Register(t *testing.T) {
	k := kernel.NewKernel()
	p := NewTasksPlugin()
	cmds, err := p.Register(k)
	require.NoError(t, err)
	require.Len(t, cmds, 1)
	assert.Equal(t, "tasks", cmds[0].Use)
}

func TestTasksPlugin_RegisterSubcommands(t *testing.T) {
	k := kernel.NewKernel()
	p := NewTasksPlugin()
	cmds, err := p.Register(k)
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	root := cmds[0]
	subNames := map[string]bool{}
	for _, sub := range root.Commands() {
		subNames[sub.Use] = true
	}

	expected := []string{"list", "prioritize", "infer-sessions", "daily", "weekly", "timesheet"}
	for _, name := range expected {
		assert.True(t, subNames[name], "missing subcommand: %s", name)
	}
}

func TestFilterTasks_NoFilter(t *testing.T) {
	tasks := []Task{
		{ID: "T001", Status: StatusTodo, Project: "a"},
		{ID: "T002", Status: StatusInProgress, Project: "b"},
	}
	result := filterTasks(tasks, "", "")
	assert.Len(t, result, 2)
}

func TestFilterTasks_ByStatus(t *testing.T) {
	tasks := []Task{
		{ID: "T001", Status: StatusTodo},
		{ID: "T002", Status: StatusInProgress},
	}
	result := filterTasks(tasks, "todo", "")
	assert.Len(t, result, 1)
	assert.Equal(t, "T001", result[0].ID)
}

func TestFilterTasks_ByProject(t *testing.T) {
	tasks := []Task{
		{ID: "T001", Status: StatusTodo, Project: "shared-devops"},
		{ID: "T002", Status: StatusTodo, Project: "chapsvision"},
	}
	result := filterTasks(tasks, "", "shared-devops")
	assert.Len(t, result, 1)
	assert.Equal(t, "T001", result[0].ID)
}

func TestFilterTasks_CaseInsensitiveProject(t *testing.T) {
	tasks := []Task{{ID: "T001", Status: StatusTodo, Project: "Shared-Devops"}}
	result := filterTasks(tasks, "", "shared-devops")
	assert.Len(t, result, 1)
}

func TestParseJSON_StripCodeBlock(t *testing.T) {
	text := "```json\n{\"effort\":\"medium\"}\n```"
	var v struct{ Effort string `json:"effort"` }
	err := parseJSON(text, &v)
	assert.NoError(t, err)
	assert.Equal(t, "medium", v.Effort)
}

func TestParseJSON_WithPreamble(t *testing.T) {
	text := "Sure! Here is the JSON:\n{\"effort\":\"large\"}"
	var v struct{ Effort string `json:"effort"` }
	err := parseJSON(text, &v)
	assert.NoError(t, err)
	assert.Equal(t, "large", v.Effort)
}

func TestParseJSON_NoJSON(t *testing.T) {
	var v struct{}
	err := parseJSON("no json here", &v)
	assert.Error(t, err)
}

func TestFormatMinutes_Zero(t *testing.T) {
	assert.Equal(t, "0h00", FormatMinutes(0))
}

func TestFormatTaskID(t *testing.T) {
	assert.Equal(t, "T001", FormatTaskID(1))
	assert.Equal(t, "T047", FormatTaskID(47))
	assert.Equal(t, "T100", FormatTaskID(100))
}
