package tasks

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	reHeading   = regexp.MustCompile(`^##\s+(T\d+)\s*·\s*(.+)$`)
	reBoldField = regexp.MustCompile(`^\*\*([^*]+)\*\*\s*(.*)$`)
	reTimeLog   = regexp.MustCompile(`^-\s+(\d{4}-\d{2}-\d{2}):\s+(\d+)min?\s+\[(\w+)\]\s+\[(\w+)\](?:\s+(.*))?$`)
	reDurHM     = regexp.MustCompile(`^(\d+)h(\d+)(?:m(?:in)?)?$`)
	reDurH      = regexp.MustCompile(`^(\d+)h$`)
	reDurM      = regexp.MustCompile(`^(\d+)m(?:in)?$`)
	reDurN      = regexp.MustCompile(`^(\d+)$`)
)

// ParseTaskFromContent parses a Mempalace drawer's Markdown content into a Task.
func ParseTaskFromContent(drawerID, content string) (Task, error) {
	task := Task{DrawerID: drawerID}

	type section int
	const (
		secHeader section = iota
		secFields
		secDescription
		secSubtasks
		secTimeLogs
	)

	cur := secHeader
	var descLines []string

	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimRight(raw, " \t\r")
		lower := strings.ToLower(line)

		if strings.HasPrefix(line, "### ") {
			switch {
			case strings.Contains(lower, "sous-t"):
				cur = secSubtasks
			case strings.Contains(lower, "time log"):
				cur = secTimeLogs
			}
			continue
		}

		switch cur {
		case secHeader:
			if m := reHeading.FindStringSubmatch(line); m != nil {
				task.ID = m[1]
				task.Title = strings.TrimSpace(m[2])
				cur = secFields
			}

		case secFields:
			if line == "" {
				continue
			}
			if m := reBoldField.FindStringSubmatch(line); m != nil {
				key := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(m[1]), ":"))
				val := strings.TrimSpace(m[2])
				applyField(&task, key, val)
			} else {
				// First non-field non-empty line → start of description
				cur = secDescription
				descLines = append(descLines, line)
			}

		case secDescription:
			descLines = append(descLines, line)

		case secSubtasks:
			if line != "" {
				task.Subtasks = append(task.Subtasks, line)
			}

		case secTimeLogs:
			if line == "" || !strings.HasPrefix(line, "-") {
				continue
			}
			if tl, ok := parseTimeLogLine(line); ok {
				task.TimeLogs = append(task.TimeLogs, tl)
			}
		}
	}

	for len(descLines) > 0 && strings.TrimSpace(descLines[len(descLines)-1]) == "" {
		descLines = descLines[:len(descLines)-1]
	}
	if len(descLines) > 0 {
		task.Description = strings.Join(descLines, "\n")
	}

	if task.ID == "" {
		return Task{}, fmt.Errorf("no task ID found in drawer content")
	}
	return task, nil
}

func applyField(task *Task, key, val string) {
	switch strings.ToLower(key) {
	case "projet", "project":
		task.Project = val
	case "objectif", "objective":
		task.Objective = val
	case "statut", "status":
		task.Status = parseStatus(val)
	case "priorité", "priorite", "priority":
		task.Priority = val
	case "eisenhower":
		if len(val) >= 2 {
			q := val[:2]
			switch Eisenhower(q) {
			case EisenhowerQ1, EisenhowerQ2, EisenhowerQ3, EisenhowerQ4:
				task.Eisenhower = Eisenhower(q)
			}
		}
	case "catégorie", "categorie", "category":
		task.Category = parseCategory(val)
	case "effort":
		task.Effort = parseEffort(val)
	case "impact":
		task.Impact = parseImpact(val)
	case "score":
		if n, err := strconv.Atoi(val); err == nil {
			task.PriorityScore = n
		}
	case "deadline":
		task.Deadline = val
	case "type":
		task.TaskType = val
	case "dépendances", "dependances", "dependencies":
		for _, p := range strings.Split(val, ",") {
			if p = strings.TrimSpace(p); p != "" {
				task.Dependencies = append(task.Dependencies, p)
			}
		}
	}
}

func parseStatus(s string) Status {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "in_progress":
		return StatusInProgress
	case "blocked_external":
		return StatusBlockedExternal
	case "monitoring":
		return StatusMonitoring
	case "pending_decision":
		return StatusPendingDecision
	case "backlog":
		return StatusBacklog
	case "done":
		return StatusDone
	default:
		return StatusTodo
	}
}

func parseCategory(s string) Category {
	if strings.ToLower(strings.TrimSpace(s)) == "management" {
		return CategoryManagement
	}
	return CategoryDev
}

func parseEffort(s string) Effort {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "medium":
		return EffortMedium
	case "large":
		return EffortLarge
	default:
		return EffortSmall
	}
}

func parseImpact(s string) Impact {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "medium":
		return ImpactMedium
	case "high":
		return ImpactHigh
	default:
		return ImpactLow
	}
}

func parseTimeLogLine(line string) (TimeLog, bool) {
	// Structured: - 2026-07-01: 90min [work] [manual] Note
	if m := reTimeLog.FindStringSubmatch(line); m != nil {
		dur, err := strconv.Atoi(m[2])
		if err != nil {
			return TimeLog{}, false
		}
		return TimeLog{
			Date:            m[1],
			DurationMinutes: dur,
			Type:            m[3],
			Source:          m[4],
			Note:            strings.TrimSpace(m[5]),
		}, true
	}
	// Legacy: - 2026-07-01: Dev
	trimmed := strings.TrimPrefix(line, "- ")
	parts := strings.SplitN(trimmed, ": ", 2)
	if len(parts) == 2 {
		date := strings.TrimSpace(parts[0])
		if len(date) == 10 && date[4] == '-' && date[7] == '-' {
			return TimeLog{
				Date:   date,
				Type:   "work",
				Source: "manual",
				Note:   strings.TrimSpace(parts[1]),
			}, true
		}
	}
	return TimeLog{}, false
}

// FormatTaskContent serializes a Task to Markdown for storage in Mempalace.
func FormatTaskContent(task Task) string {
	var b strings.Builder

	fmt.Fprintf(&b, "## %s · %s\n\n", task.ID, task.Title)

	if task.Project != "" {
		fmt.Fprintf(&b, "**Projet :** %s\n", task.Project)
	}
	if task.Objective != "" {
		fmt.Fprintf(&b, "**Objectif :** %s\n", task.Objective)
	}
	fmt.Fprintf(&b, "**Statut :** %s\n", task.Status)
	if task.Priority != "" {
		fmt.Fprintf(&b, "**Priorité :** %s\n", task.Priority)
	}
	if task.Eisenhower != "" {
		fmt.Fprintf(&b, "**Eisenhower :** %s — %s\n", task.Eisenhower, eisenhowerLabel(task.Eisenhower))
	}
	if task.Category != "" {
		fmt.Fprintf(&b, "**Catégorie :** %s\n", task.Category)
	}
	if task.Effort != "" {
		fmt.Fprintf(&b, "**Effort :** %s\n", task.Effort)
	}
	if task.Impact != "" {
		fmt.Fprintf(&b, "**Impact :** %s\n", task.Impact)
	}
	if task.PriorityScore > 0 {
		fmt.Fprintf(&b, "**Score :** %d\n", task.PriorityScore)
	}
	if task.Deadline != "" {
		fmt.Fprintf(&b, "**Deadline :** %s\n", task.Deadline)
	}
	if len(task.Dependencies) > 0 {
		fmt.Fprintf(&b, "**Dépendances :** %s\n", strings.Join(task.Dependencies, ", "))
	}
	if task.TaskType != "" {
		fmt.Fprintf(&b, "**Type :** %s\n", task.TaskType)
	}

	if task.Description != "" {
		b.WriteString("\n")
		b.WriteString(task.Description)
		b.WriteString("\n")
	}

	if len(task.Subtasks) > 0 {
		b.WriteString("\n### Sous-tâches\n")
		for _, st := range task.Subtasks {
			b.WriteString(st)
			b.WriteString("\n")
		}
	}

	if len(task.TimeLogs) > 0 {
		b.WriteString("\n### Time logs\n")
		for _, l := range task.TimeLogs {
			if l.DurationMinutes > 0 {
				fmt.Fprintf(&b, "- %s: %dmin [%s] [%s]", l.Date, l.DurationMinutes, l.Type, l.Source)
			} else {
				fmt.Fprintf(&b, "- %s: [%s] [%s]", l.Date, l.Type, l.Source)
			}
			if l.Note != "" {
				fmt.Fprintf(&b, " %s", l.Note)
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

func eisenhowerLabel(e Eisenhower) string {
	switch e {
	case EisenhowerQ1:
		return "Urgent + Important"
	case EisenhowerQ2:
		return "Important, pas urgent"
	case EisenhowerQ3:
		return "Urgent, pas important"
	case EisenhowerQ4:
		return "Ni urgent ni important"
	}
	return ""
}

// ParseDuration parses human-readable duration strings like "1h30", "90min", "2h", "30m".
func ParseDuration(s string) (int, error) {
	s = strings.TrimSpace(strings.ToLower(strings.ReplaceAll(s, " ", "")))
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	if m := reDurHM.FindStringSubmatch(s); m != nil {
		h, _ := strconv.Atoi(m[1])
		min, _ := strconv.Atoi(m[2])
		if h*60+min == 0 {
			return 0, fmt.Errorf("zero duration")
		}
		return h*60 + min, nil
	}
	if m := reDurH.FindStringSubmatch(s); m != nil {
		h, _ := strconv.Atoi(m[1])
		if h == 0 {
			return 0, fmt.Errorf("zero duration")
		}
		return h * 60, nil
	}
	if m := reDurM.FindStringSubmatch(s); m != nil {
		min, _ := strconv.Atoi(m[1])
		if min == 0 {
			return 0, fmt.Errorf("zero duration")
		}
		return min, nil
	}
	if m := reDurN.FindStringSubmatch(s); m != nil {
		min, _ := strconv.Atoi(m[1])
		if min == 0 {
			return 0, fmt.Errorf("zero duration")
		}
		return min, nil
	}
	return 0, fmt.Errorf("invalid duration %q (expected e.g. 1h30, 90min, 2h, 30m)", s)
}

// parseTaskNumber extracts the numeric part from a T-number string (e.g. "T047" → 47).
func parseTaskNumber(id string) int {
	id = strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(id)), "T")
	n, _ := strconv.Atoi(id)
	return n
}

// FormatTaskID formats a task number as a T-number string.
func FormatTaskID(n int) string {
	return fmt.Sprintf("T%03d", n)
}
