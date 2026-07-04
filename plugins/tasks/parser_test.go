package tasks

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTaskFromContent_Minimal(t *testing.T) {
	content := "## T001 · My task\n\n**Statut :** todo\n"
	task, err := ParseTaskFromContent("drawer_tasks_board_abc123", content)
	require.NoError(t, err)
	assert.Equal(t, "T001", task.ID)
	assert.Equal(t, "My task", task.Title)
	assert.Equal(t, StatusTodo, task.Status)
	assert.Equal(t, "drawer_tasks_board_abc123", task.DrawerID)
}

func TestParseTaskFromContent_AllFields(t *testing.T) {
	content := `## T047 · Renovate — Work Item créé avec le compte de service

**Projet :** shared-devops
**Objectif :** Obj 2
**Statut :** in_progress
**Priorité :** HAUTE
**Eisenhower :** Q2 — Important, pas urgent
**Catégorie :** dev
**Effort :** medium
**Impact :** high
**Score :** 85
**Deadline :** 2026-07-10
**Dépendances :** T041, T046
**Type :** SUIVI ÉQUIPE

La création des Work Items ADO par Renovate doit utiliser le compte de service.

### Sous-tâches
- [ ] Créer le compte de service
- [x] Vérifier les permissions

### Time logs
- 2026-07-01: 90min [work] [manual] Review code
- 2026-07-02: 30min [meeting] [inferred]
`
	task, err := ParseTaskFromContent("drawer_tasks_board_xyz", content)
	require.NoError(t, err)

	assert.Equal(t, "T047", task.ID)
	assert.Equal(t, "Renovate — Work Item créé avec le compte de service", task.Title)
	assert.Equal(t, "shared-devops", task.Project)
	assert.Equal(t, "Obj 2", task.Objective)
	assert.Equal(t, StatusInProgress, task.Status)
	assert.Equal(t, "HAUTE", task.Priority)
	assert.Equal(t, EisenhowerQ2, task.Eisenhower)
	assert.Equal(t, CategoryDev, task.Category)
	assert.Equal(t, EffortMedium, task.Effort)
	assert.Equal(t, ImpactHigh, task.Impact)
	assert.Equal(t, 85, task.PriorityScore)
	assert.Equal(t, "2026-07-10", task.Deadline)
	assert.Equal(t, []string{"T041", "T046"}, task.Dependencies)
	assert.Equal(t, "SUIVI ÉQUIPE", task.TaskType)
	assert.Contains(t, task.Description, "La création des Work Items")
	assert.Len(t, task.Subtasks, 2)
	assert.Len(t, task.TimeLogs, 2)
	assert.Equal(t, "2026-07-01", task.TimeLogs[0].Date)
	assert.Equal(t, 90, task.TimeLogs[0].DurationMinutes)
	assert.Equal(t, "work", task.TimeLogs[0].Type)
	assert.Equal(t, "manual", task.TimeLogs[0].Source)
	assert.Equal(t, "Review code", task.TimeLogs[0].Note)
	assert.Equal(t, "2026-07-02", task.TimeLogs[1].Date)
	assert.Equal(t, 30, task.TimeLogs[1].DurationMinutes)
	assert.Equal(t, "meeting", task.TimeLogs[1].Type)
	assert.Equal(t, "inferred", task.TimeLogs[1].Source)
}

func TestParseTaskFromContent_LegacyTimeLog(t *testing.T) {
	content := `## T041 · Test

**Statut :** in_progress

### Time logs
- 2026-07-01: Dev
`
	task, err := ParseTaskFromContent("d1", content)
	require.NoError(t, err)
	require.Len(t, task.TimeLogs, 1)
	assert.Equal(t, "2026-07-01", task.TimeLogs[0].Date)
	assert.Equal(t, "Dev", task.TimeLogs[0].Note)
	assert.Equal(t, "work", task.TimeLogs[0].Type)
}

func TestParseTaskFromContent_NoID(t *testing.T) {
	_, err := ParseTaskFromContent("d1", "no heading here")
	assert.Error(t, err)
}

func TestParseTaskFromContent_StatusValues(t *testing.T) {
	cases := []struct {
		raw      string
		expected Status
	}{
		{"todo", StatusTodo},
		{"in_progress", StatusInProgress},
		{"blocked_external", StatusBlockedExternal},
		{"monitoring", StatusMonitoring},
		{"pending_decision", StatusPendingDecision},
		{"backlog", StatusBacklog},
		{"done", StatusDone},
		{"unknown", StatusTodo},
	}
	for _, tc := range cases {
		content := "## T001 · T\n\n**Statut :** " + tc.raw + "\n"
		task, err := ParseTaskFromContent("d", content)
		require.NoError(t, err, tc.raw)
		assert.Equal(t, tc.expected, task.Status, tc.raw)
	}
}

func TestFormatTaskContent_RoundTrip(t *testing.T) {
	task := Task{
		ID:            "T047",
		DrawerID:      "drawer_tasks_board_abc",
		Title:         "Renovate — Work Item",
		Project:       "shared-devops",
		Objective:     "Obj 2",
		Status:        StatusInProgress,
		Priority:      "HAUTE",
		Eisenhower:    EisenhowerQ2,
		Category:      CategoryDev,
		Effort:        EffortMedium,
		Impact:        ImpactHigh,
		PriorityScore: 85,
		Deadline:      "2026-07-10",
		Dependencies:  []string{"T041", "T046"},
		Description:   "La création des Work Items ADO.",
		Subtasks:      []string{"- [ ] Créer le compte", "- [x] Vérifier"},
		TimeLogs: []TimeLog{
			{Date: "2026-07-01", DurationMinutes: 90, Type: "work", Source: "manual", Note: "Review"},
		},
	}

	content := FormatTaskContent(task)
	parsed, err := ParseTaskFromContent(task.DrawerID, content)
	require.NoError(t, err)

	assert.Equal(t, task.ID, parsed.ID)
	assert.Equal(t, task.Title, parsed.Title)
	assert.Equal(t, task.Project, parsed.Project)
	assert.Equal(t, task.Objective, parsed.Objective)
	assert.Equal(t, task.Status, parsed.Status)
	assert.Equal(t, task.Priority, parsed.Priority)
	assert.Equal(t, task.Eisenhower, parsed.Eisenhower)
	assert.Equal(t, task.Category, parsed.Category)
	assert.Equal(t, task.Effort, parsed.Effort)
	assert.Equal(t, task.Impact, parsed.Impact)
	assert.Equal(t, task.PriorityScore, parsed.PriorityScore)
	assert.Equal(t, task.Deadline, parsed.Deadline)
	assert.Equal(t, task.Dependencies, parsed.Dependencies)
	assert.Equal(t, task.Subtasks, parsed.Subtasks)
	require.Len(t, parsed.TimeLogs, 1)
	assert.Equal(t, task.TimeLogs[0].Date, parsed.TimeLogs[0].Date)
	assert.Equal(t, task.TimeLogs[0].DurationMinutes, parsed.TimeLogs[0].DurationMinutes)
	assert.Equal(t, task.TimeLogs[0].Note, parsed.TimeLogs[0].Note)
}

func TestFormatTaskContent_ContainsHeading(t *testing.T) {
	task := Task{ID: "T001", Title: "My title", Status: StatusTodo}
	content := FormatTaskContent(task)
	assert.True(t, strings.HasPrefix(content, "## T001 · My title"))
}

func TestParseDuration(t *testing.T) {
	cases := []struct {
		input    string
		expected int
		hasErr   bool
	}{
		{"1h30", 90, false},
		{"2h", 120, false},
		{"30m", 30, false},
		{"30min", 30, false},
		{"90", 90, false},
		{"1h30m", 90, false},
		{"1h30min", 90, false},
		{"0h30", 30, false},
		{"", 0, true},
		{"abc", 0, true},
	}
	for _, tc := range cases {
		got, err := ParseDuration(tc.input)
		if tc.hasErr {
			assert.Error(t, err, tc.input)
		} else {
			assert.NoError(t, err, tc.input)
			assert.Equal(t, tc.expected, got, tc.input)
		}
	}
}

func TestTaskTotalMinutes(t *testing.T) {
	task := Task{
		TimeLogs: []TimeLog{
			{DurationMinutes: 90},
			{DurationMinutes: 30},
		},
	}
	assert.Equal(t, 120, task.TotalMinutes())
}

func TestTaskTotalMinutesForDate(t *testing.T) {
	task := Task{
		TimeLogs: []TimeLog{
			{Date: "2026-07-01", DurationMinutes: 90},
			{Date: "2026-07-02", DurationMinutes: 30},
			{Date: "2026-07-01", DurationMinutes: 60},
		},
	}
	assert.Equal(t, 150, task.TotalMinutesForDate("2026-07-01"))
	assert.Equal(t, 30, task.TotalMinutesForDate("2026-07-02"))
	assert.Equal(t, 0, task.TotalMinutesForDate("2026-07-03"))
}

func TestTaskIsActive(t *testing.T) {
	assert.True(t, Task{Status: StatusTodo}.IsActive())
	assert.True(t, Task{Status: StatusInProgress}.IsActive())
	assert.False(t, Task{Status: StatusDone}.IsActive())
	assert.False(t, Task{Status: StatusBacklog}.IsActive())
}

func TestTaskIsBlocked(t *testing.T) {
	assert.True(t, Task{Status: StatusBlockedExternal}.IsBlocked())
	assert.True(t, Task{Status: StatusPendingDecision}.IsBlocked())
	assert.False(t, Task{Status: StatusTodo}.IsBlocked())
}

func TestParseCategory(t *testing.T) {
	assert.Equal(t, CategoryManagement, parseCategory("management"))
	assert.Equal(t, CategoryManagement, parseCategory("Management"))
	assert.Equal(t, CategoryDev, parseCategory("dev"))
	assert.Equal(t, CategoryDev, parseCategory("unknown"))
	assert.Equal(t, CategoryDev, parseCategory(""))
}

func TestParseEffort(t *testing.T) {
	assert.Equal(t, EffortSmall, parseEffort("small"))
	assert.Equal(t, EffortMedium, parseEffort("medium"))
	assert.Equal(t, EffortLarge, parseEffort("large"))
	assert.Equal(t, EffortSmall, parseEffort(""))
	assert.Equal(t, EffortSmall, parseEffort("unknown"))
}

func TestParseImpact(t *testing.T) {
	assert.Equal(t, ImpactLow, parseImpact("low"))
	assert.Equal(t, ImpactMedium, parseImpact("medium"))
	assert.Equal(t, ImpactHigh, parseImpact("high"))
	assert.Equal(t, ImpactLow, parseImpact(""))
	assert.Equal(t, ImpactLow, parseImpact("unknown"))
}

func TestEisenhowerLabel(t *testing.T) {
	assert.Equal(t, "Urgent + Important", eisenhowerLabel(EisenhowerQ1))
	assert.Equal(t, "Important, pas urgent", eisenhowerLabel(EisenhowerQ2))
	assert.Equal(t, "Urgent, pas important", eisenhowerLabel(EisenhowerQ3))
	assert.Equal(t, "Ni urgent ni important", eisenhowerLabel(EisenhowerQ4))
	assert.Equal(t, "", eisenhowerLabel(""))
}

func TestParseTaskNumber(t *testing.T) {
	assert.Equal(t, 47, parseTaskNumber("T047"))
	assert.Equal(t, 1, parseTaskNumber("T001"))
	assert.Equal(t, 100, parseTaskNumber("T100"))
	assert.Equal(t, 0, parseTaskNumber(""))
	assert.Equal(t, 0, parseTaskNumber("invalid"))
}

func TestFormatTaskContent_AllEisenhowerLabels(t *testing.T) {
	for _, q := range []Eisenhower{EisenhowerQ1, EisenhowerQ2, EisenhowerQ3, EisenhowerQ4} {
		task := Task{ID: "T001", Title: "T", Status: StatusTodo, Eisenhower: q}
		content := FormatTaskContent(task)
		assert.Contains(t, content, string(q), string(q))
	}
}

func TestParseTaskFromContent_Eisenhower_AllQuadrants(t *testing.T) {
	for _, q := range []Eisenhower{EisenhowerQ1, EisenhowerQ2, EisenhowerQ3, EisenhowerQ4} {
		content := fmt.Sprintf("## T001 · T\n\n**Eisenhower :** %s — label\n", q)
		task, err := ParseTaskFromContent("d", content)
		require.NoError(t, err, string(q))
		assert.Equal(t, q, task.Eisenhower, string(q))
	}
}

func TestFormatTaskContent_TimeLogZeroDuration(t *testing.T) {
	task := Task{
		ID: "T001", Title: "T", Status: StatusTodo,
		TimeLogs: []TimeLog{{Date: "2026-07-01", DurationMinutes: 0, Type: "work", Source: "manual"}},
	}
	content := FormatTaskContent(task)
	assert.Contains(t, content, "2026-07-01")
	assert.NotContains(t, content, "0min")
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "abc", truncate("abc", 10))
	assert.Equal(t, "abc...", truncate("abcdef", 3))
}
