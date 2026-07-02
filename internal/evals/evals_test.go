package evals

import (
	"context"
	"os"
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

func TestRun_LiveSuiteDoesNotIncludeAuthenticatedE2EByDefault(t *testing.T) {
	report, err := Run(context.Background(), Options{
		WorkDir:     t.TempDir(),
		IncludeLive: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, scenario := range report.ScenarioRuns {
		if scenario.ID == "authenticated-webui-e2e" {
			t.Fatal("authenticated-webui-e2e should require IncludeAuthE2E")
		}
	}
}

func TestRun_AuthenticatedE2ERequiresSessionMaterial(t *testing.T) {
	t.Setenv("AGENTOS_EVAL_AUTH_COOKIE", "")
	t.Setenv("AGENTOS_EVAL_AUTH_STORAGE_STATE", "")
	report, err := Run(context.Background(), Options{
		WorkDir:        t.TempDir(),
		ScenarioIDs:    []string{"authenticated-webui-e2e"},
		IncludeAuthE2E: true,
		LiveURL:        "https://agentos.example.invalid",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Total != 1 || report.Passed != 0 || report.Failed != 1 {
		t.Fatalf("report = %+v, want one failing auth E2E scenario", report)
	}
	reasons := strings.Join(report.ScenarioRuns[0].FailureReasons, "\n")
	if !strings.Contains(reasons, "authenticated session material is required") {
		t.Fatalf("failure reasons = %q, want missing session material", reasons)
	}
}

func TestRun_StorageCleanupE2ERequiresCookie(t *testing.T) {
	t.Setenv("AGENTOS_EVAL_AUTH_COOKIE", "")
	report, err := Run(context.Background(), Options{
		WorkDir:                  t.TempDir(),
		ScenarioIDs:              []string{"storage-cleanup-e2e"},
		IncludeStorageCleanupE2E: true,
		LiveURL:                  "https://agentos.example.invalid",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Total != 1 || report.Passed != 0 || report.Failed != 1 {
		t.Fatalf("report = %+v, want one failing storage cleanup E2E scenario", report)
	}
	reasons := strings.Join(report.ScenarioRuns[0].FailureReasons, "\n")
	if !strings.Contains(reasons, "AGENTOS_EVAL_AUTH_COOKIE") {
		t.Fatalf("failure reasons = %q, want missing cookie", reasons)
	}
}

func TestRun_ScheduleNotificationE2ERequiresCookie(t *testing.T) {
	t.Setenv("AGENTOS_EVAL_AUTH_COOKIE", "")
	report, err := Run(context.Background(), Options{
		WorkDir:                  t.TempDir(),
		ScenarioIDs:              []string{"schedule-notification-e2e"},
		IncludeScheduleNotifyE2E: true,
		LiveURL:                  "https://agentos.example.invalid",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Total != 1 || report.Passed != 0 || report.Failed != 1 {
		t.Fatalf("report = %+v, want one failing schedule notification E2E scenario", report)
	}
	reasons := strings.Join(report.ScenarioRuns[0].FailureReasons, "\n")
	if !strings.Contains(reasons, "AGENTOS_EVAL_AUTH_COOKIE") {
		t.Fatalf("failure reasons = %q, want missing cookie", reasons)
	}
}

func TestRun_GitHubWorkflowE2ERequiresRepo(t *testing.T) {
	t.Setenv("AGENTOS_EVAL_GITHUB_REPO", "")
	report, err := Run(context.Background(), Options{
		WorkDir:                  t.TempDir(),
		ScenarioIDs:              []string{"github-workflow-e2e"},
		IncludeGitHubWorkflowE2E: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Total != 1 || report.Passed != 0 || report.Failed != 1 {
		t.Fatalf("report = %+v, want one failing GitHub workflow E2E scenario", report)
	}
	reasons := strings.Join(report.ScenarioRuns[0].FailureReasons, "\n")
	if !strings.Contains(reasons, "AGENTOS_EVAL_GITHUB_REPO") {
		t.Fatalf("failure reasons = %q, want missing repo", reasons)
	}
}

func TestRun_KubernetesRolloutE2ERequiresExplicitConfig(t *testing.T) {
	t.Setenv("AGENTOS_EVAL_KUBECONFIG", "")
	t.Setenv("AGENTOS_EVAL_KUBE_CONTEXT", "")
	t.Setenv("AGENTOS_EVAL_KUBE_NAMESPACE", "")
	report, err := Run(context.Background(), Options{
		WorkDir:                     t.TempDir(),
		ScenarioIDs:                 []string{"kubernetes-rollout-e2e"},
		IncludeKubernetesRolloutE2E: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Total != 1 || report.Passed != 0 || report.Failed != 1 {
		t.Fatalf("report = %+v, want one failing Kubernetes rollout scenario", report)
	}
	reasons := strings.Join(report.ScenarioRuns[0].FailureReasons, "\n")
	for _, want := range []string{"AGENTOS_EVAL_KUBECONFIG", "AGENTOS_EVAL_KUBE_CONTEXT", "AGENTOS_EVAL_KUBE_NAMESPACE"} {
		if !strings.Contains(reasons, want) {
			t.Fatalf("failure reasons = %q, want %s", reasons, want)
		}
	}
}

func TestSanitizeAuthE2EOutput(t *testing.T) {
	t.Setenv("AGENTOS_EVAL_AUTH_COOKIE", "agentos_session=secret")
	got := sanitizeAuthE2EOutput("failed with agentos_session=secret")
	if strings.Contains(got, "secret") || !strings.Contains(got, "[redacted]") {
		t.Fatalf("sanitizeAuthE2EOutput() = %q", got)
	}
}

func TestSanitizeKubernetesOutput(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_secret")
	got := sanitizeKubernetesOutput("event included ghp_secret")
	if strings.Contains(got, "ghp_secret") || !strings.Contains(got, "[redacted]") {
		t.Fatalf("sanitizeKubernetesOutput() = %q", got)
	}
}

func TestHasInboxDelivery(t *testing.T) {
	deliveries := []notificationEvalDelivery{{Destination: "inbox", Status: "success"}}
	if !hasInboxDelivery(deliveries) {
		t.Fatal("hasInboxDelivery() = false, want true")
	}
	if hasInboxDelivery([]notificationEvalDelivery{{Destination: "webhook", Status: "success"}}) {
		t.Fatal("hasInboxDelivery() = true for non-inbox delivery, want false")
	}
}

func TestSplitGitHubRepo(t *testing.T) {
	owner, name, ok := splitGitHubRepo("owner/repo.git")
	if !ok || owner != "owner" || name != "repo" {
		t.Fatalf("splitGitHubRepo() = %q %q %v, want owner repo true", owner, name, ok)
	}
	if _, _, ok := splitGitHubRepo("owner/repo/extra"); ok {
		t.Fatal("splitGitHubRepo() accepted nested path")
	}
}

func TestHasCleanupAuditEvent(t *testing.T) {
	summary := storageCleanupEvalSummary{Selected: 1, Archived: 1, Deleted: 0, Skipped: 1}
	events := []storageAuditEvalEvent{{
		Action:  "storage.cleanup",
		Outcome: "success",
		Target:  "storage",
		Message: "selected=1 archived=1 deleted=0 skipped=1",
	}}
	if !hasCleanupAuditEvent(events, summary) {
		t.Fatal("hasCleanupAuditEvent() = false, want true")
	}
	if hasCleanupAuditEvent(events, storageCleanupEvalSummary{Selected: 2, Archived: 1, Deleted: 0, Skipped: 1}) {
		t.Fatal("hasCleanupAuditEvent() = true for mismatched summary, want false")
	}
}

func TestFindAuthE2EScriptOverride(t *testing.T) {
	script := t.TempDir() + "/auth-e2e.mjs"
	if err := os.WriteFile(script, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTOS_EVAL_AUTH_E2E_SCRIPT", script)
	got, err := findAuthE2EScript()
	if err != nil {
		t.Fatalf("findAuthE2EScript() error = %v", err)
	}
	if got != script {
		t.Fatalf("script = %q, want %q", got, script)
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

func TestMarkdown_IncludesScenarioChecks(t *testing.T) {
	report := &Report{
		Total:       1,
		Passed:      1,
		SuccessRate: 1,
		ScenarioRuns: []ScenarioResult{{
			ID:     "authenticated-webui-e2e",
			Name:   "Authenticated Web UI E2E",
			Mode:   ModePlan,
			Passed: true,
			Checks: []ScenarioCheck{{
				Page:       "mobile",
				Action:     "bottom navigation layout",
				Passed:     true,
				DurationMS: 120,
			}},
		}},
	}
	out := Markdown(report)
	for _, want := range []string{"Scenario Checks", "mobile", "bottom navigation layout"} {
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
