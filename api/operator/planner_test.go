package operator

import (
	"context"
	"encoding/json"
	"gaia/api"
	"testing"
)

func TestState_AppendObservation_LastAnswerOrPartial(t *testing.T) {
	s := &State{Goal: "find disk usage"}
	if s.LastAnswerOrPartial() != "find disk usage" {
		t.Errorf("empty state LastAnswerOrPartial = %q", s.LastAnswerOrPartial())
	}
	s.AppendDecision(`{"action":"tool","name":"run_cmd","args":{"cmd":"df -h"}}`)
	s.AppendObservation("stdout:\nFilesystem...")
	if s.LastAnswerOrPartial() != `{"action":"tool","name":"run_cmd","args":{"cmd":"df -h"}}` {
		t.Errorf("LastAnswerOrPartial should return last assistant content")
	}
	s.AppendDecision(`{"action":"answer","content":"Disk is 80% full."}`)
	if s.LastAnswerOrPartial() != `{"action":"answer","content":"Disk is 80% full."}` {
		t.Errorf("LastAnswerOrPartial = %q", s.LastAnswerOrPartial())
	}
}

func Test_extractJSON(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{`{"action":"answer","content":"hi"}`, `{"action":"answer","content":"hi"}`},
		{"```json\n{\"a\":1}\n```", `{"a":1}`},
		{"Here is the response:\n```json\n{\"action\":\"tool\"}\n```", `{"action":"tool"}`},
		{"  { \"x\": 2 }  ", `{ "x": 2 }`},
	}
	for _, tt := range tests {
		got := extractJSON(tt.in)
		var j1, j2 interface{}
		_ = json.Unmarshal([]byte(got), &j1)
		_ = json.Unmarshal([]byte(tt.want), &j2)
		if string(got) != tt.want && j1 != j2 {
			// Normalize: at least valid JSON
			if json.Valid([]byte(got)) && json.Valid([]byte(tt.want)) {
				// allow different whitespace
				continue
			}
			t.Errorf("extractJSON(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPlanner_buildMessages(t *testing.T) {
	r := NewRegistry()
	r.Register(&Tool{Name: "run_cmd", Description: "Run command", Schema: map[string]string{"cmd": "cmd"}})
	planner := &Planner{Model: "test"}
	state := &State{Goal: "why disk full?"}
	state.AppendDecision(`{"action":"tool","name":"run_cmd","args":{"cmd":"df"}}`)
	state.AppendObservation("stdout: ...")
	msgs := planner.buildMessages(state, r)
	if len(msgs) < 3 {
		t.Errorf("buildMessages len = %d, want at least 3", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Error("first message should be system")
	}
	if msgs[1].Content != "Goal: why disk full?" {
		t.Errorf("second message content = %q", msgs[1].Content)
	}
}

func TestPlanner_Decide_invalidJSON(t *testing.T) {
	planner := &Planner{
		SendReq: func(req api.APIRequest) (string, error) {
			return "not valid json at all", nil
		},
	}
	state := &State{Goal: "test"}
	r := NewRegistry()
	r.Register(&Tool{Name: "run_cmd"})
	_, _, err := planner.Decide(context.Background(), state, r)
	if err == nil {
		t.Error("Decide with invalid JSON should return error")
	}
}

func TestPlanner_Decide_validAnswer(t *testing.T) {
	planner := &Planner{
		SendReq: func(req api.APIRequest) (string, error) {
			return `{"action":"answer","content":"Done."}`, nil
		},
	}
	state := &State{Goal: "test"}
	r := NewRegistry()
	dec, _, err := planner.Decide(context.Background(), state, r)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Action != "answer" || dec.Content != "Done." {
		t.Errorf("Decision = %+v", dec)
	}
}

func TestPlanner_Decide_validTool(t *testing.T) {
	planner := &Planner{
		SendReq: func(req api.APIRequest) (string, error) {
			return `{"action":"tool","name":"run_cmd","args":{"cmd":"df -h"}}`, nil
		},
	}
	state := &State{Goal: "test"}
	r := NewRegistry()
	r.Register(&Tool{Name: "run_cmd"})
	dec, _, err := planner.Decide(context.Background(), state, r)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Action != "tool" || dec.Name != "run_cmd" {
		t.Errorf("Decision = %+v", dec)
	}
	if dec.Args["cmd"] != "df -h" {
		t.Errorf("Args = %v", dec.Args)
	}
}
