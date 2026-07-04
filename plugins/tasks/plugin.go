package tasks

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"gaia/kernel"
	"gaia/plugins/mempalace"
	"gaia/plugins/shared"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// TasksPlugin manages tasks stored in Mempalace.
type TasksPlugin struct{}

func NewTasksPlugin() *TasksPlugin { return &TasksPlugin{} }

func (p *TasksPlugin) ID() string            { return "tasks" }
func (p *TasksPlugin) DefaultEnabled() bool  { return true }
func (p *TasksPlugin) DependsOn() []string   { return []string{"mempalace"} }
func (p *TasksPlugin) ConfigSchema() []string {
	return []string{
		"tasks.ollama_host",
		"tasks.ollama_port",
		"tasks.model",
	}
}

func (p *TasksPlugin) Register(k *kernel.Kernel) ([]*cobra.Command, error) {
	root := &cobra.Command{
		Use:   "tasks",
		Short: "Manage and prioritize tasks (Mempalace-backed)",
	}

	root.AddCommand(
		p.listCmd(),
		p.addCmd(),
		p.updateCmd(),
		p.doneCmd(),
		p.logCmd(),
		p.prioritizeCmd(),
		p.inferSessionsCmd(),
		p.dailyCmd(),
		p.weeklyCmd(),
		p.timesheetCmd(),
	)
	return []*cobra.Command{root}, nil
}

// --- list ---

func (p *TasksPlugin) listCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks (board view by default)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store := NewStore()
			tasks, err := store.ListAll(cmd.Context())
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}
			flat, _ := cmd.Flags().GetBool("flat")
			status, _ := cmd.Flags().GetString("status")
			project, _ := cmd.Flags().GetString("projet")

			filtered := filterTasks(tasks, status, project)
			view := BoardView(filtered, flat)
			if strings.TrimSpace(view) == "" {
				fmt.Fprintln(cmd.OutOrStdout(), "No active tasks found.")
				return nil
			}
			fmt.Fprint(cmd.OutOrStdout(), view)
			return nil
		},
	}
	cmd.Flags().Bool("flat", false, "Flat list sorted by priority score")
	cmd.Flags().String("status", "", "Filter by status (todo, in_progress, blocked_external…)")
	cmd.Flags().String("projet", "", "Filter by project")
	return cmd
}

// --- add ---

func (p *TasksPlugin) addCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new task (interactive or via flags)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			title, _ := cmd.Flags().GetString("title")
			project, _ := cmd.Flags().GetString("projet")
			deadline, _ := cmd.Flags().GetString("deadline")
			eisenhower, _ := cmd.Flags().GetString("eisenhower")

			// Interactive fallback for missing required fields
			if title == "" {
				title = prompt("Titre de la task : ")
			}
			if title == "" {
				return shared.PrintError(cmd.ErrOrStderr(), "Le titre est requis")
			}
			if project == "" {
				project = prompt("Projet (ex: shared-devops) : ")
			}

			store := NewStore()
			maxID, err := store.NextID(cmd.Context())
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("NextID: %v", err))
			}

			task := Task{
				ID:         FormatTaskID(maxID + 1),
				Title:      title,
				Status:     StatusTodo,
				Project:    project,
				Deadline:   deadline,
				Eisenhower: Eisenhower(strings.ToUpper(eisenhower)),
			}

			// LLM inference
			llm := newLLMClient(cmd)
			fmt.Fprintf(cmd.ErrOrStderr(), "Inférence LLM en cours…\n")
			meta, err := llm.InferTaskMeta(cmd.Context(), task)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "[warn] LLM inférence échouée: %v\n", err)
			} else {
				task.Effort = meta.Effort
				task.Impact = meta.Impact
				task.Category = meta.Category
				task.PriorityScore = meta.PriorityScore
				if task.Eisenhower == "" {
					task.Eisenhower = meta.Eisenhower
				}
			}

			created, err := store.Add(cmd.Context(), task)
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Ajout: %v", err))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Task %s créée (drawer: %s)\n", created.ID, created.DrawerID)
			if meta.PriorityScore > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "  Score: %d | %s | effort: %s | impact: %s | category: %s\n",
					meta.PriorityScore, task.Eisenhower, task.Effort, task.Impact, task.Category)
			}
			return nil
		},
	}
	cmd.Flags().String("title", "", "Task title")
	cmd.Flags().String("projet", "", "Project name")
	cmd.Flags().String("deadline", "", "Deadline (YYYY-MM-DD)")
	cmd.Flags().String("eisenhower", "", "Eisenhower quadrant (Q1/Q2/Q3/Q4)")
	return cmd
}

// --- update ---

func (p *TasksPlugin) updateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <task-id>",
		Short: "Update a task's fields",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store := NewStore()
			task, err := store.Get(cmd.Context(), args[0])
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}

			if title, _ := cmd.Flags().GetString("title"); title != "" {
				task.Title = title
			}
			if status, _ := cmd.Flags().GetString("status"); status != "" {
				task.Status = parseStatus(status)
			}
			if deadline, _ := cmd.Flags().GetString("deadline"); deadline != "" {
				task.Deadline = deadline
			}
			if priority, _ := cmd.Flags().GetString("priority"); priority != "" {
				task.Priority = strings.ToUpper(priority)
			}

			if err := store.Update(cmd.Context(), task); err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Update: %v", err))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Task %s mise à jour.\n", task.ID)
			return nil
		},
	}
	cmd.Flags().String("title", "", "New title")
	cmd.Flags().String("status", "", "New status")
	cmd.Flags().String("deadline", "", "New deadline (YYYY-MM-DD)")
	cmd.Flags().String("priority", "", "New priority (HAUTE/MOYENNE/BASSE)")
	return cmd
}

// --- done ---

func (p *TasksPlugin) doneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "done <task-id>",
		Short: "Mark a task as done",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store := NewStore()
			task, err := store.Get(cmd.Context(), args[0])
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}
			task.Status = StatusDone
			if err := store.Update(cmd.Context(), task); err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Done: %v", err))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Task %s marquée comme done.\n", task.ID)
			return nil
		},
	}
}

// --- log ---

func (p *TasksPlugin) logCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log <task-id> <duration>",
		Short: "Log time on a task (e.g. 1h30, 90min, 2h)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			mins, err := ParseDuration(args[1])
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Durée invalide: %v", err))
			}
			logType, _ := cmd.Flags().GetString("type")
			if logType == "" {
				logType = "work"
			}
			note, _ := cmd.Flags().GetString("note")
			date, _ := cmd.Flags().GetString("date")
			if date == "" {
				date = time.Now().Format("2006-01-02")
			}

			store := NewStore()
			if err := store.AddTimeLog(cmd.Context(), args[0], TimeLog{
				Date:            date,
				DurationMinutes: mins,
				Type:            logType,
				Source:          "manual",
				Note:            note,
			}); err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Log: %v", err))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Loggué %s sur %s (%s)\n", FormatMinutes(mins), args[0], date)
			return nil
		},
	}
	cmd.Flags().String("type", "work", "Log type: work or meeting")
	cmd.Flags().String("note", "", "Optional note")
	cmd.Flags().String("date", "", "Date (YYYY-MM-DD, default: today)")
	return cmd
}

// --- prioritize ---

func (p *TasksPlugin) prioritizeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prioritize",
		Short: "Re-compute priority scores for all active tasks",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store := NewStore()
			tasks, err := store.ListAll(cmd.Context())
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}

			active := make([]Task, 0, len(tasks))
			for _, t := range tasks {
				if t.IsActive() {
					active = append(active, t)
				}
			}
			if len(active) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Aucune task active.")
				return nil
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "Priorisation de %d tasks via LLM…\n", len(active))
			llm := newLLMClient(cmd)
			today := time.Now().Format("2006-01-02")
			result, err := llm.Prioritize(cmd.Context(), active, today)
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("LLM: %v", err))
			}

			// Apply scores back
			byID := map[string]PrioritizedTask{}
			for _, pt := range result.Tasks {
				byID[pt.ID] = pt
			}
			for _, task := range active {
				pt, ok := byID[task.ID]
				if !ok {
					continue
				}
				task.PriorityScore = pt.PriorityScore
				task.Eisenhower = pt.Eisenhower
				task.Effort = pt.Effort
				task.Impact = pt.Impact
				task.Category = pt.Category
				if err := store.Update(cmd.Context(), task); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "[warn] update %s: %v\n", task.ID, err)
				}
			}

			if result.Narrative != "" {
				_ = shared.PrintBox(cmd.OutOrStdout(), "Priorités", result.Narrative)
			}
			return nil
		},
	}
}

// --- infer-sessions ---

func (p *TasksPlugin) inferSessionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "infer-sessions",
		Short: "Infer time logs from Claude Code session transcripts",
		RunE: func(cmd *cobra.Command, _ []string) error {
			sinceStr, _ := cmd.Flags().GetString("since")
			var since time.Time
			if sinceStr != "" {
				var err error
				since, err = time.Parse("2006-01-02", sinceStr)
				if err != nil {
					return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Date invalide: %v", err))
				}
			}

			projectsDir := ClaudeProjectsDir()
			sessions, err := ReadSessions(projectsDir, since)
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Lecture sessions: %v", err))
			}
			if len(sessions) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Aucune session à inférer.")
				return nil
			}

			store := NewStore()
			tasks, err := store.ListAll(cmd.Context())
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "Inférence de %d sessions via LLM…\n", len(sessions))
			llm := newLLMClient(cmd)
			entries, err := llm.InferSessions(cmd.Context(), tasks, sessions)
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("LLM: %v", err))
			}

			for _, e := range entries {
				if err := store.AddTimeLog(cmd.Context(), e.TaskID, TimeLog{
					Date:            e.Date,
					DurationMinutes: e.DurationMinutes,
					Type:            "work",
					Source:          "inferred",
				}); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "[warn] log %s: %v\n", e.TaskID, err)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Loggué %dmin sur %s (%s, confidence=%.0f%%)\n",
					e.DurationMinutes, e.TaskID, e.Date, e.Confidence*100)
			}
			return nil
		},
	}
	cmd.Flags().String("since", "", "Only process sessions after this date (YYYY-MM-DD)")
	return cmd
}

// --- daily ---

func (p *TasksPlugin) dailyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "daily",
		Short: "Generate daily standup report",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store := NewStore()
			tasks, err := store.ListAll(cmd.Context())
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}

			today := time.Now().Format("2006-01-02")
			yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

			// Try to get journal
			journal := ""
			if raw, jerr := mempalace.CallTool(cmd.Context(), "mempalace_diary_read", map[string]interface{}{}); jerr == nil {
				var result struct{ Content string `json:"content"` }
				if err := unmarshalMCP(raw, &result); err == nil {
					journal = result.Content
				}
			}

			report := DailyReport(tasks, today, yesterday, journal)
			fmt.Fprint(cmd.OutOrStdout(), report)
			return nil
		},
	}
}

// --- weekly ---

func (p *TasksPlugin) weeklyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "weekly",
		Short: "Generate weekly progress report",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store := NewStore()
			tasks, err := store.ListAll(cmd.Context())
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}

			now := time.Now()
			// Find Monday of current week
			weekday := int(now.Weekday())
			if weekday == 0 {
				weekday = 7
			}
			monday := now.AddDate(0, 0, -(weekday - 1))
			friday := monday.AddDate(0, 0, 4)
			from := monday.Format("2006-01-02")
			to := friday.Format("2006-01-02")

			report := WeeklyReport(tasks, from, to)
			fmt.Fprint(cmd.OutOrStdout(), report)
			return nil
		},
	}
}

// --- timesheet ---

func (p *TasksPlugin) timesheetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "timesheet",
		Short: "Generate timesheet for copy-paste into the time tracking tool",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store := NewStore()
			tasks, err := store.ListAll(cmd.Context())
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}

			now := time.Now()
			weekStr, _ := cmd.Flags().GetString("week")
			var monday time.Time
			if weekStr != "" {
				d, err := time.Parse("2006-01-02", weekStr)
				if err != nil {
					return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Date invalide: %v", err))
				}
				monday = d
			} else {
				weekday := int(now.Weekday())
				if weekday == 0 {
					weekday = 7
				}
				monday = now.AddDate(0, 0, -(weekday - 1))
			}
			friday := monday.AddDate(0, 0, 4)
			from := monday.Format("2006-01-02")
			to := friday.Format("2006-01-02")

			report := TimesheetReport(tasks, from, to)
			fmt.Fprint(cmd.OutOrStdout(), report)
			return nil
		},
	}
	cmd.Flags().String("week", "", "Week start (Monday, YYYY-MM-DD, default: current week)")
	return cmd
}

// --- helpers ---

func newLLMClient(cmd *cobra.Command) *LLMClient {
	host := viper.GetString("tasks.ollama_host")
	if host == "" {
		host = viper.GetString("ask.host")
	}
	if host == "" {
		host = "localhost"
	}
	port := viper.GetInt("tasks.ollama_port")
	if port == 0 {
		port = viper.GetInt("ask.port")
	}
	if port == 0 {
		port = 11434
	}
	model := viper.GetString("tasks.model")
	if model == "" {
		model = defaultModel
	}
	c := NewLLMClient(host, port)
	c.model = model
	_ = cmd
	return c
}

func filterTasks(tasks []Task, status, project string) []Task {
	if status == "" && project == "" {
		return tasks
	}
	out := tasks[:0]
	for _, t := range tasks {
		if status != "" && string(t.Status) != status {
			continue
		}
		if project != "" && !strings.EqualFold(t.Project, project) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func prompt(label string) string {
	fmt.Print(label)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}
