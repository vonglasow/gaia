package tasks

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var fixtureTasksForReport = []Task{
	{
		ID: "T001", Title: "Urgent fix", Status: StatusInProgress,
		Category: CategoryDev, Priority: "HAUTE", Eisenhower: EisenhowerQ1,
		TimeLogs: []TimeLog{
			{Date: "2026-07-02", DurationMinutes: 120, Type: "work", Source: "manual"},
			{Date: "2026-07-03", DurationMinutes: 60, Type: "work", Source: "manual"},
		},
	},
	{
		ID: "T002", Title: "Management meeting", Status: StatusTodo,
		Category: CategoryManagement, Priority: "MOYENNE", Eisenhower: EisenhowerQ2,
		TimeLogs: []TimeLog{
			{Date: "2026-07-02", DurationMinutes: 30, Type: "meeting", Source: "manual"},
		},
	},
	{
		ID: "T003", Title: "Blocked task", Status: StatusBlockedExternal,
		Category: CategoryDev, Priority: "HAUTE",
	},
	{
		ID: "T004", Title: "Done task", Status: StatusDone,
		Category: CategoryDev,
		TimeLogs: []TimeLog{
			{Date: "2026-07-02", DurationMinutes: 60, Type: "work", Source: "manual"},
		},
	},
}

func TestDailyReport_ContainsSections(t *testing.T) {
	report := DailyReport(fixtureTasksForReport, "2026-07-03", "2026-07-02", "")
	assert.Contains(t, report, "Hier")
	assert.Contains(t, report, "Aujourd'hui")
	assert.Contains(t, report, "Blocages")
}

func TestDailyReport_YesterdayTasks(t *testing.T) {
	report := DailyReport(fixtureTasksForReport, "2026-07-03", "2026-07-02", "")
	assert.Contains(t, report, "T001")
	assert.Contains(t, report, "T002")
	// T004 (done) with time log on 2026-07-02 should appear in yesterday
	assert.Contains(t, report, "T004")
}

func TestDailyReport_TodayInProgress(t *testing.T) {
	report := DailyReport(fixtureTasksForReport, "2026-07-03", "2026-07-02", "")
	// T001 is in_progress so it should appear in today section
	assert.Contains(t, report, "T001")
}

func TestDailyReport_Blocages(t *testing.T) {
	report := DailyReport(fixtureTasksForReport, "2026-07-03", "2026-07-02", "")
	assert.Contains(t, report, "T003")
}

func TestDailyReport_JournalEnrichment(t *testing.T) {
	journal := "Hier j'ai livré le script de publication APP."
	report := DailyReport(fixtureTasksForReport, "2026-07-03", "2026-07-02", journal)
	assert.Contains(t, report, "script de publication")
}

func TestWeeklyReport_ContainsObjectives(t *testing.T) {
	tasks := []Task{
		{ID: "T001", Title: "Cartographie", Status: StatusInProgress, Objective: "Obj 1"},
		{ID: "T002", Title: "Renovate", Status: StatusDone, Objective: "Obj 2"},
	}
	report := WeeklyReport(tasks, "2026-06-30", "2026-07-04")
	assert.Contains(t, report, "Obj 1")
	assert.Contains(t, report, "Obj 2")
	assert.Contains(t, report, "Cartographie")
	assert.Contains(t, report, "Renovate")
}

func TestWeeklyReport_DateRange(t *testing.T) {
	report := WeeklyReport(fixtureTasksForReport, "2026-06-29", "2026-07-03")
	assert.Contains(t, report, "2026-06-29")
	assert.Contains(t, report, "2026-07-03")
}

func TestTimesheetReport_Structure(t *testing.T) {
	report := TimesheetReport(fixtureTasksForReport, "2026-06-29", "2026-07-03")
	assert.Contains(t, report, "Dev")
	assert.Contains(t, report, "Management")
	// Should have 5 weekday columns
	assert.Contains(t, report, "Lun")
	assert.Contains(t, report, "Mar")
	assert.Contains(t, report, "Mer")
	assert.Contains(t, report, "Jeu")
	assert.Contains(t, report, "Ven")
	assert.Contains(t, report, "TOTAL")
}

func TestTimesheetReport_Hours(t *testing.T) {
	tasks := []Task{
		{
			ID: "T001", Title: "Dev work", Category: CategoryDev,
			TimeLogs: []TimeLog{
				{Date: "2026-06-29", DurationMinutes: 120}, // lundi = 2h
				{Date: "2026-06-30", DurationMinutes: 90},  // mardi = 1h30
			},
		},
		{
			ID: "T002", Title: "Management", Category: CategoryManagement,
			TimeLogs: []TimeLog{
				{Date: "2026-06-29", DurationMinutes: 60}, // lundi = 1h
			},
		},
	}
	report := TimesheetReport(tasks, "2026-06-29", "2026-07-03")
	assert.Contains(t, report, "2h00") // 120 min = 2h
	assert.Contains(t, report, "1h30") // 90 min = 1h30
	assert.Contains(t, report, "1h00") // 60 min = 1h
}

func TestFormatMinutes(t *testing.T) {
	cases := []struct {
		minutes  int
		expected string
	}{
		{0, "0h00"},
		{60, "1h00"},
		{90, "1h30"},
		{150, "2h30"},
		{480, "8h00"},
	}
	for _, tc := range cases {
		got := FormatMinutes(tc.minutes)
		assert.Equal(t, tc.expected, got, "minutes=%d", tc.minutes)
	}
}

func TestWeekdays(t *testing.T) {
	// 2026-06-29=Mon … 2026-07-03=Fri → 5 weekdays
	days := weekdays("2026-06-29", "2026-07-03")
	assert.Len(t, days, 5)
	assert.Equal(t, "2026-06-29", days[0])
	assert.Equal(t, "2026-07-03", days[4])
}

func TestWeekdays_SkipsWeekend(t *testing.T) {
	// 2026-07-04=Sat, 2026-07-05=Sun are skipped; Mon starts 2026-07-06
	days := weekdays("2026-07-04", "2026-07-10")
	assert.Len(t, days, 5)
	assert.Equal(t, "2026-07-06", days[0]) // Monday
}

func TestBoardView(t *testing.T) {
	tasks := []Task{
		{ID: "T001", Title: "Urgent", Status: StatusInProgress, Eisenhower: EisenhowerQ1, PriorityScore: 90},
		{ID: "T002", Title: "Important", Status: StatusTodo, Eisenhower: EisenhowerQ2, PriorityScore: 60},
		{ID: "T003", Title: "Backlog", Status: StatusBacklog, Eisenhower: EisenhowerQ4, PriorityScore: 10},
	}
	view := BoardView(tasks, false)
	assert.Contains(t, view, "T001")
	assert.Contains(t, view, "T002")
	// Filter out backlog by default
	assert.NotContains(t, view, "T003")
}

func TestBoardView_Flat(t *testing.T) {
	tasks := []Task{
		{ID: "T001", Status: StatusInProgress, PriorityScore: 90, Title: "A"},
		{ID: "T002", Status: StatusTodo, PriorityScore: 40, Title: "B"},
	}
	view := BoardView(tasks, true)
	// Should list by score desc
	idxT1 := strings.Index(view, "T001")
	idxT2 := strings.Index(view, "T002")
	assert.Less(t, idxT1, idxT2)
}
