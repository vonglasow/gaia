package investigate

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRun_emptyGoal(t *testing.T) {
	_, err := Run(context.Background(), "  ", RunOptions{ShellRunner: &mockShellRunner{}})
	if err == nil {
		t.Error("Run(empty goal) should return error")
	}
}

func TestRun_errorComparable(t *testing.T) {
	if !errors.Is(ErrMaxStepsReached, ErrMaxStepsReached) {
		t.Error("ErrMaxStepsReached should be comparable with errors.Is")
	}
}

func TestState_AppendDecision_AppendObservation(t *testing.T) {
	s := &State{Goal: "g"}
	s.AppendDecision(`{"action":"tool","name":"run_cmd","args":{"cmd":"x"}}`)
	s.AppendObservation("result")
	if len(s.Steps) != 2 {
		t.Errorf("Steps len = %d", len(s.Steps))
	}
	if s.Steps[0].Role != "assistant" || s.Steps[1].Role != "user" {
		t.Errorf("Steps = %+v", s.Steps)
	}
}

func TestLastAnswerOrPartial(t *testing.T) {
	s := &State{Goal: "my goal"}
	if got := s.LastAnswerOrPartial(); got != "my goal" {
		t.Errorf("LastAnswerOrPartial() = %q, want %q", got, "my goal")
	}
	s.Steps = append(s.Steps, Step{Role: "assistant", Content: "last"})
	if got := s.LastAnswerOrPartial(); got != "last" {
		t.Errorf("LastAnswerOrPartial() = %q, want %q", got, "last")
	}
}

func Test_extractJSON_braceMatch(t *testing.T) {
	in := `before {"a":{"b":1}} after`
	got := extractJSON(in)
	if !strings.Contains(got, "a") {
		t.Errorf("extractJSON nested = %q", got)
	}
}
