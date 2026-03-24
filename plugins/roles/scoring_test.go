package roles

import "testing"

func TestScoreText_SelectRole(t *testing.T) {
	rules := ScoringConfig{
		"code":  {Keywords: []string{"code", "bug"}, Weight: 1.5},
		"shell": {Keywords: []string{"shell"}, Weight: 1.0},
	}
	scores := ScoreText("fix a bug in code", rules)
	res := SelectRole(scores, 1.0, "shell")
	if res.RoleName != "code" {
		t.Errorf("RoleName = %q, want code", res.RoleName)
	}
	if !res.Matched {
		t.Error("expected Matched=true")
	}
}

func TestSelectRole_Default(t *testing.T) {
	scores := map[string]float64{"default": 0.0}
	res := SelectRole(scores, 1.0, "default")
	if res.RoleName != "default" {
		t.Errorf("RoleName = %q, want default", res.RoleName)
	}
	if res.Matched {
		t.Error("expected Matched=false")
	}
}
