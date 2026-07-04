package tasks

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// DailyReport generates a daily standup report.
// yesterday and today are YYYY-MM-DD strings.
func DailyReport(tasks []Task, today, yesterday, journal string) string {
	var b strings.Builder

	// Hier
	b.WriteString("**Hier :**\n")
	for _, t := range tasks {
		if t.TotalMinutesForDate(yesterday) > 0 {
			fmt.Fprintf(&b, "- %s %s\n", t.ID, t.Title)
		}
	}
	if strings.TrimSpace(journal) != "" {
		// Append first line of journal as additional context
		lines := strings.Split(strings.TrimSpace(journal), "\n")
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l != "" && !strings.HasPrefix(l, "#") {
				fmt.Fprintf(&b, "- %s\n", l)
				break
			}
		}
	}

	// Aujourd'hui
	b.WriteString("\n**Aujourd'hui :**\n")
	for _, t := range tasks {
		if t.Status == StatusInProgress {
			fmt.Fprintf(&b, "- %s %s\n", t.ID, t.Title)
			continue
		}
		// Include high-priority todo tasks
		if t.Status == StatusTodo && t.Priority == "HAUTE" {
			if t.Deadline != "" {
				fmt.Fprintf(&b, "- %s %s (deadline %s)\n", t.ID, t.Title, t.Deadline)
			} else {
				fmt.Fprintf(&b, "- %s %s\n", t.ID, t.Title)
			}
		}
	}

	// Blocages
	b.WriteString("\n**Blocages :**\n")
	hasBlocker := false
	for _, t := range tasks {
		if t.IsBlocked() {
			fmt.Fprintf(&b, "- %s %s\n", t.ID, t.Title)
			hasBlocker = true
		}
	}
	if !hasBlocker {
		b.WriteString("- Aucun\n")
	}

	return b.String()
}

// WeeklyReport generates a weekly progress report grouped by objective.
func WeeklyReport(tasks []Task, from, to string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Weekly — %s au %s\n\n", from, to)

	// Group by objective
	byObj := map[string][]Task{}
	noObj := []Task{}
	for _, t := range tasks {
		if t.Objective != "" {
			byObj[t.Objective] = append(byObj[t.Objective], t)
		} else {
			noObj = append(noObj, t)
		}
	}

	// Sort objectives
	objs := make([]string, 0, len(byObj))
	for o := range byObj {
		objs = append(objs, o)
	}
	sort.Strings(objs)

	for _, obj := range objs {
		fmt.Fprintf(&b, "### %s\n", obj)
		for _, t := range byObj[obj] {
			icon := statusIcon(t.Status)
			fmt.Fprintf(&b, "- %s %s %s\n", icon, t.ID, t.Title)
		}
		b.WriteString("\n")
	}

	if len(noObj) > 0 {
		b.WriteString("### Divers\n")
		for _, t := range noObj {
			icon := statusIcon(t.Status)
			fmt.Fprintf(&b, "- %s %s %s\n", icon, t.ID, t.Title)
		}
	}

	return b.String()
}

var frDayAbbr = map[time.Weekday]string{
	time.Monday:    "Lun",
	time.Tuesday:   "Mar",
	time.Wednesday: "Mer",
	time.Thursday:  "Jeu",
	time.Friday:    "Ven",
}

// TimesheetReport generates a copy-paste ready timesheet table.
// from/to are YYYY-MM-DD; only weekdays in range are included.
func TimesheetReport(tasks []Task, from, to string) string {
	days := weekdays(from, to)
	dayNames := make([]string, len(days))
	for i, d := range days {
		t, _ := time.Parse("2006-01-02", d)
		dayNames[i] = frDayAbbr[t.Weekday()]
	}

	// Accumulate minutes per category per day
	devMins := make([]int, len(days))
	mgmtMins := make([]int, len(days))

	for _, t := range tasks {
		for di, day := range days {
			mins := t.TotalMinutesForDate(day)
			if mins == 0 {
				continue
			}
			if t.Category == CategoryManagement {
				mgmtMins[di] += mins
			} else {
				devMins[di] += mins
			}
		}
	}

	// Build table
	var b strings.Builder
	fmt.Fprintf(&b, "Semaine du %s au %s\n\n", from, to)

	// Header
	b.WriteString("           ")
	for _, name := range dayNames[:len(days)] {
		fmt.Fprintf(&b, "| %-6s", name)
	}
	b.WriteString("| TOTAL\n")

	sep := strings.Repeat("-", 11+len(days)*8+8)
	b.WriteString(sep + "\n")

	// Dev row
	devTotal := 0
	b.WriteString("Dev        ")
	for _, m := range devMins {
		fmt.Fprintf(&b, "| %-6s", FormatMinutes(m))
		devTotal += m
	}
	fmt.Fprintf(&b, "| %s\n", FormatMinutes(devTotal))

	// Management row
	mgmtTotal := 0
	b.WriteString("Management ")
	for _, m := range mgmtMins {
		fmt.Fprintf(&b, "| %-6s", FormatMinutes(m))
		mgmtTotal += m
	}
	fmt.Fprintf(&b, "| %s\n", FormatMinutes(mgmtTotal))

	b.WriteString(sep + "\n")

	// Total row
	b.WriteString("Total      ")
	for i := range days {
		fmt.Fprintf(&b, "| %-6s", FormatMinutes(devMins[i]+mgmtMins[i]))
	}
	fmt.Fprintf(&b, "| %s\n", FormatMinutes(devTotal+mgmtTotal))

	return b.String()
}

// BoardView renders tasks as a board (grouped by Eisenhower) or flat list (sorted by score).
func BoardView(tasks []Task, flat bool) string {
	active := make([]Task, 0, len(tasks))
	for _, t := range tasks {
		if t.IsActive() {
			active = append(active, t)
		}
	}

	if flat {
		sort.Slice(active, func(i, j int) bool {
			return active[i].PriorityScore > active[j].PriorityScore
		})
		var b strings.Builder
		for _, t := range active {
			fmt.Fprintf(&b, "[%3d] %s · %s [%s] %s\n",
				t.PriorityScore, t.ID, t.Title, t.Status, t.Priority)
		}
		return b.String()
	}

	// Board view grouped by Eisenhower
	quadrants := []struct {
		q     Eisenhower
		label string
	}{
		{EisenhowerQ1, "Q1 — Urgent + Important (FAIRE MAINTENANT)"},
		{EisenhowerQ2, "Q2 — Important, pas urgent (PLANIFIER)"},
		{EisenhowerQ3, "Q3 — Urgent, pas important (DÉLÉGUER)"},
		{EisenhowerQ4, "Q4 — Ni urgent ni important (BATCHER)"},
	}

	var b strings.Builder
	for _, quad := range quadrants {
		var qTasks []Task
		for _, t := range active {
			if t.Eisenhower == quad.q {
				qTasks = append(qTasks, t)
			}
		}
		if len(qTasks) == 0 {
			continue
		}
		sort.Slice(qTasks, func(i, j int) bool {
			return qTasks[i].PriorityScore > qTasks[j].PriorityScore
		})
		fmt.Fprintf(&b, "\n## %s\n\n", quad.label)
		for _, t := range qTasks {
			fmt.Fprintf(&b, "- [%3d] %s · %s [%s] %s\n",
				t.PriorityScore, t.ID, t.Title, t.Status, t.Priority)
		}
	}

	// Tasks without a quadrant
	var noQ []Task
	for _, t := range active {
		if t.Eisenhower == "" {
			noQ = append(noQ, t)
		}
	}
	if len(noQ) > 0 {
		b.WriteString("\n## Autres\n\n")
		for _, t := range noQ {
			fmt.Fprintf(&b, "- %s · %s [%s]\n", t.ID, t.Title, t.Status)
		}
	}

	return b.String()
}

// FormatMinutes formats a duration in minutes as "Xh00".
func FormatMinutes(minutes int) string {
	h := minutes / 60
	m := minutes % 60
	return fmt.Sprintf("%dh%02d", h, m)
}

// weekdays returns the list of weekday dates (Mon-Fri) between from and to (inclusive).
func weekdays(from, to string) []string {
	start, err := time.Parse("2006-01-02", from)
	if err != nil {
		return nil
	}
	end, err := time.Parse("2006-01-02", to)
	if err != nil {
		return nil
	}
	var days []string
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			days = append(days, d.Format("2006-01-02"))
		}
	}
	return days
}

func statusIcon(s Status) string {
	switch s {
	case StatusDone:
		return "[x]"
	case StatusInProgress:
		return "[>]"
	case StatusBlockedExternal:
		return "[!]"
	case StatusPendingDecision:
		return "[?]"
	case StatusMonitoring:
		return "[~]"
	default:
		return "[ ]"
	}
}
