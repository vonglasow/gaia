package tasks

// Status represents the lifecycle state of a task.
type Status string

const (
	StatusTodo            Status = "todo"
	StatusInProgress      Status = "in_progress"
	StatusBlockedExternal Status = "blocked_external"
	StatusMonitoring      Status = "monitoring"
	StatusPendingDecision Status = "pending_decision"
	StatusBacklog         Status = "backlog"
	StatusDone            Status = "done"
)

// Eisenhower quadrant.
type Eisenhower string

const (
	EisenhowerQ1 Eisenhower = "Q1" // urgent + important
	EisenhowerQ2 Eisenhower = "Q2" // important, pas urgent
	EisenhowerQ3 Eisenhower = "Q3" // urgent, pas important
	EisenhowerQ4 Eisenhower = "Q4" // ni urgent ni important
)

// Category drives timesheet split (dev vs. management).
type Category string

const (
	CategoryDev        Category = "dev"
	CategoryManagement Category = "management"
)

// Effort is LLM-inferred size of a task.
type Effort string

const (
	EffortSmall  Effort = "small"
	EffortMedium Effort = "medium"
	EffortLarge  Effort = "large"
)

// Impact is LLM-inferred business value of a task.
type Impact string

const (
	ImpactLow    Impact = "low"
	ImpactMedium Impact = "medium"
	ImpactHigh   Impact = "high"
)

// TimeLog records one work session on a task.
type TimeLog struct {
	Date            string `json:"date"` // YYYY-MM-DD
	DurationMinutes int    `json:"duration_minutes"`
	Type            string `json:"type"`   // "work" | "meeting"
	Source          string `json:"source"` // "manual" | "inferred"
	Note            string `json:"note,omitempty"`
}

// Task is the core domain object stored as a Mempalace drawer.
type Task struct {
	ID            string     `json:"id"`
	DrawerID      string     `json:"drawer_id,omitempty"`
	Title         string     `json:"title"`
	Description   string     `json:"description,omitempty"`
	Status        Status     `json:"status"`
	Priority      string     `json:"priority,omitempty"` // HAUTE | MOYENNE | BASSE
	Eisenhower    Eisenhower `json:"eisenhower,omitempty"`
	Category      Category   `json:"category,omitempty"`
	Effort        Effort     `json:"effort,omitempty"`
	Impact        Impact     `json:"impact,omitempty"`
	PriorityScore int        `json:"priority_score,omitempty"` // 0-100, LLM-computed
	Deadline      string     `json:"deadline,omitempty"`       // YYYY-MM-DD
	Project       string     `json:"project,omitempty"`
	Objective     string     `json:"objective,omitempty"`
	Dependencies  []string   `json:"dependencies,omitempty"`
	Subtasks      []string   `json:"subtasks,omitempty"`
	TimeLogs      []TimeLog  `json:"time_logs,omitempty"`
	TaskType      string     `json:"task_type,omitempty"` // "SUIVI ÉQUIPE" etc.
	Narrative     string     `json:"narrative,omitempty"` // LLM global narrative (not per-task)
}

// TotalMinutes returns total logged time for this task.
func (t Task) TotalMinutes() int {
	total := 0
	for _, l := range t.TimeLogs {
		total += l.DurationMinutes
	}
	return total
}

// TotalMinutesForDate returns minutes logged on a specific date.
func (t Task) TotalMinutesForDate(date string) int {
	total := 0
	for _, l := range t.TimeLogs {
		if l.Date == date {
			total += l.DurationMinutes
		}
	}
	return total
}

// IsActive returns true if the task is not done or backlog.
func (t Task) IsActive() bool {
	return t.Status != StatusDone && t.Status != StatusBacklog
}

// IsBlocked returns true if the task is waiting on an external dependency.
func (t Task) IsBlocked() bool {
	return t.Status == StatusBlockedExternal || t.Status == StatusPendingDecision
}
