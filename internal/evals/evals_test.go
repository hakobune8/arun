package evals

import (
	"context"
	"strings"
	"testing"
)

func TestRun_DefaultSuite(t *testing.T) {
	report, err := Run(context.Background(), Options{WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Total != len(DefaultScenarios()) {
		t.Fatalf("total = %d, want %d", report.Total, len(DefaultScenarios()))
	}
	if report.Failed != 0 || report.Passed != report.Total {
		t.Fatalf("report = %+v, want all scenarios passing", report)
	}
	if report.SuccessRate != 1 {
		t.Fatalf("success rate = %f, want 1", report.SuccessRate)
	}
	if len(report.Coverage) == 0 {
		t.Fatal("coverage summary is empty")
	}
}

func TestRun_ExecuteScenarioReportsArtifacts(t *testing.T) {
	report, err := Run(context.Background(), Options{
		WorkDir:     t.TempDir(),
		ScenarioIDs: []string{"empty-go-service-bootstrap"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Total != 1 || report.Failed != 0 {
		t.Fatalf("report = %+v, want one passing scenario", report)
	}
	result := report.ScenarioRuns[0]
	for _, want := range []string{"go-backend", "docs", "ci-fixer", "reviewer"} {
		if !contains(result.Agents, want) {
			t.Fatalf("agents = %+v, want %q", result.Agents, want)
		}
	}
	if result.Successes != 4 || result.Failures != 0 {
		t.Fatalf("successes=%d failures=%d, want 4/0", result.Successes, result.Failures)
	}
	for _, file := range result.RequiredFiles {
		if !file.Exists {
			t.Fatalf("required file missing: %+v", file)
		}
	}
	if result.Artifacts["diff"] == "" || !strings.Contains(result.Artifacts["diff"], "/healthz") {
		t.Fatalf("diff artifact missing /healthz: %+v", result.Artifacts)
	}
}

func TestMarkdown_IncludesFailures(t *testing.T) {
	report := &Report{
		Total:       1,
		Failed:      1,
		SuccessRate: 0,
		ScenarioRuns: []ScenarioResult{{
			ID:             "scenario",
			Name:           "Scenario",
			Mode:           ModePlan,
			Agents:         []string{"docs"},
			ExpectedAgents: []string{"docs"},
			FailureReasons: []string{"missing expected agent"},
		}},
	}
	out := Markdown(report)
	for _, want := range []string{"Orchestration Eval Report", "Functional Coverage", "scenario", "missing expected agent"} {
		if !strings.Contains(out, want) {
			t.Fatalf("Markdown() missing %q:\n%s", want, out)
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
