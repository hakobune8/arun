package evals

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/hakobune8/arun/internal/llm"
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

func TestRun_ThreeSprintScrumScenarioReportsContinuity(t *testing.T) {
	report, err := Run(context.Background(), Options{
		WorkDir:     t.TempDir(),
		ScenarioIDs: []string{"three-sprint-agile-scrum"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Total != 1 || report.Failed != 0 || report.Passed != 1 {
		t.Fatalf("report = %+v, want one passing scrum scenario", report)
	}
	result := report.ScenarioRuns[0]
	for _, want := range []string{"analyst", "reporter", "reviewer", "qa", "release-manager"} {
		if !contains(result.Agents, want) {
			t.Fatalf("agents = %+v, want %q", result.Agents, want)
		}
	}
	if result.Artifacts["sprintCount"] != "3" || result.Artifacts["completedWork"] != "7" || result.Artifacts["blockerCount"] != "1" {
		t.Fatalf("scrum artifacts = %+v, want sprintCount=3 completedWork=7 blockerCount=1", result.Artifacts)
	}
	if !strings.Contains(result.Artifacts["summary"], `"carried":["AOS-106"]`) {
		t.Fatalf("summary = %s, want AOS-106 carried between sprints", result.Artifacts["summary"])
	}
	for _, file := range result.RequiredFiles {
		if !file.Exists {
			t.Fatalf("required scrum file missing: %+v", file)
		}
	}
}

func TestRun_ThreeSprintScrumLiveModeRequiresExplicitConfig(t *testing.T) {
	t.Setenv("ARUN_EVAL_SCRUM_LIVE", "true")
	t.Setenv("ARUN_EVAL_GITHUB_REPO", "")
	t.Setenv("ARUN_EVAL_LLM_PRESET_MATRIX", "")
	t.Setenv("ARUN_LLM_PRESETS", "")
	report, err := Run(context.Background(), Options{
		WorkDir:     t.TempDir(),
		ScenarioIDs: []string{"three-sprint-agile-scrum"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Total != 1 || report.Passed != 0 || report.Failed != 1 {
		t.Fatalf("report = %+v, want failing live scrum readiness", report)
	}
	reasons := strings.Join(report.ScenarioRuns[0].FailureReasons, "\n")
	for _, want := range []string{"ARUN_EVAL_GITHUB_REPO", "ARUN_EVAL_LLM_PRESET_MATRIX"} {
		if !strings.Contains(reasons, want) {
			t.Fatalf("failure reasons = %q, want %s", reasons, want)
		}
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
	t.Setenv("ARUN_EVAL_AUTH_COOKIE", "")
	t.Setenv("ARUN_EVAL_AUTH_STORAGE_STATE", "")
	report, err := Run(context.Background(), Options{
		WorkDir:        t.TempDir(),
		ScenarioIDs:    []string{"authenticated-webui-e2e"},
		IncludeAuthE2E: true,
		LiveURL:        "https://arun.example.invalid",
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
	t.Setenv("ARUN_EVAL_AUTH_COOKIE", "")
	report, err := Run(context.Background(), Options{
		WorkDir:                  t.TempDir(),
		ScenarioIDs:              []string{"storage-cleanup-e2e"},
		IncludeStorageCleanupE2E: true,
		LiveURL:                  "https://arun.example.invalid",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Total != 1 || report.Passed != 0 || report.Failed != 1 {
		t.Fatalf("report = %+v, want one failing storage cleanup E2E scenario", report)
	}
	reasons := strings.Join(report.ScenarioRuns[0].FailureReasons, "\n")
	if !strings.Contains(reasons, "ARUN_EVAL_AUTH_COOKIE") {
		t.Fatalf("failure reasons = %q, want missing cookie", reasons)
	}
}

func TestRun_ScheduleNotificationE2ERequiresCookie(t *testing.T) {
	t.Setenv("ARUN_EVAL_AUTH_COOKIE", "")
	report, err := Run(context.Background(), Options{
		WorkDir:                  t.TempDir(),
		ScenarioIDs:              []string{"schedule-notification-e2e"},
		IncludeScheduleNotifyE2E: true,
		LiveURL:                  "https://arun.example.invalid",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Total != 1 || report.Passed != 0 || report.Failed != 1 {
		t.Fatalf("report = %+v, want one failing schedule notification E2E scenario", report)
	}
	reasons := strings.Join(report.ScenarioRuns[0].FailureReasons, "\n")
	if !strings.Contains(reasons, "ARUN_EVAL_AUTH_COOKIE") {
		t.Fatalf("failure reasons = %q, want missing cookie", reasons)
	}
}

func TestRun_GitHubWorkflowE2ERequiresRepo(t *testing.T) {
	t.Setenv("ARUN_EVAL_GITHUB_REPO", "")
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
	if !strings.Contains(reasons, "ARUN_EVAL_GITHUB_REPO") {
		t.Fatalf("failure reasons = %q, want missing repo", reasons)
	}
}

func TestRun_ScrumGitHubE2ERequiresAllowlist(t *testing.T) {
	t.Setenv("ARUN_EVAL_GITHUB_REPO", "owner/repo")
	t.Setenv("ARUN_EVAL_GITHUB_REPO_ALLOWLIST", "owner/other")
	report, err := Run(context.Background(), Options{
		WorkDir:               t.TempDir(),
		ScenarioIDs:           []string{"three-sprint-scrum-github-e2e"},
		IncludeScrumGitHubE2E: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Total != 1 || report.Passed != 0 || report.Failed != 1 {
		t.Fatalf("report = %+v, want one failing scrum GitHub E2E scenario", report)
	}
	reasons := strings.Join(report.ScenarioRuns[0].FailureReasons, "\n")
	if !strings.Contains(reasons, "ARUN_EVAL_GITHUB_REPO_ALLOWLIST") {
		t.Fatalf("failure reasons = %q, want missing allowlist", reasons)
	}
}

func TestRun_ScrumGitHubE2ERejectsInvalidCleanupMode(t *testing.T) {
	t.Setenv("ARUN_EVAL_GITHUB_REPO", "owner/repo")
	t.Setenv("ARUN_EVAL_GITHUB_REPO_ALLOWLIST", "owner/repo")
	t.Setenv("ARUN_EVAL_SCRUM_GITHUB_CLEANUP", "delete")
	report, err := Run(context.Background(), Options{
		WorkDir:               t.TempDir(),
		ScenarioIDs:           []string{"three-sprint-scrum-github-e2e"},
		IncludeScrumGitHubE2E: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Total != 1 || report.Passed != 0 || report.Failed != 1 {
		t.Fatalf("report = %+v, want one failing scrum GitHub E2E scenario", report)
	}
	reasons := strings.Join(report.ScenarioRuns[0].FailureReasons, "\n")
	if !strings.Contains(reasons, "ARUN_EVAL_SCRUM_GITHUB_CLEANUP") {
		t.Fatalf("failure reasons = %q, want invalid cleanup mode", reasons)
	}
}

func TestRun_ScrumGitHubE2ELLMPresetsRequireMatrixBeforeToken(t *testing.T) {
	t.Setenv("ARUN_EVAL_GITHUB_REPO", "owner/repo")
	t.Setenv("ARUN_EVAL_GITHUB_REPO_ALLOWLIST", "owner/repo")
	t.Setenv("ARUN_EVAL_SCRUM_LLM_PRESETS", "true")
	t.Setenv("ARUN_EVAL_LLM_PRESET_MATRIX", "")
	t.Setenv("ARUN_LLM_PRESETS", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	report, err := Run(context.Background(), Options{
		WorkDir:               t.TempDir(),
		ScenarioIDs:           []string{"three-sprint-scrum-github-e2e"},
		IncludeScrumGitHubE2E: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Total != 1 || report.Passed != 0 || report.Failed != 1 {
		t.Fatalf("report = %+v, want one failing scrum GitHub E2E scenario", report)
	}
	reasons := strings.Join(report.ScenarioRuns[0].FailureReasons, "\n")
	if !strings.Contains(reasons, "ARUN_EVAL_LLM_PRESET_MATRIX") || strings.Contains(reasons, "GitHub token") {
		t.Fatalf("failure reasons = %q, want preset matrix failure before GitHub token lookup", reasons)
	}
}

func TestScrumLiteLLMPresetMapFromEnvRequiresFivePresets(t *testing.T) {
	t.Setenv("ARUN_EVAL_LLM_PRESET_MATRIX", `[{"id":"smoke","model":"test-model","baseUrl":"http://litellm:4000"}]`)
	_, err := scrumLiteLLMPresetMapFromEnv()
	if err == nil {
		t.Fatal("scrumLiteLLMPresetMapFromEnv() error = nil, want missing presets")
	}
	for _, want := range []string{"planning", "coding", "review", "reporting"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want missing %s", err.Error(), want)
		}
	}
}

func TestScrumLLMStageCheckReportsUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s, want /chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"review risk is recorded"}}],"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13}}`))
	}))
	defer server.Close()
	preset := liteLLMPresetEvalConfig{
		ID:            "review",
		Model:         "test-model",
		BaseURL:       server.URL,
		Timeout:       "5s",
		Temperature:   0,
		MaxTokens:     64,
		TokenBudget:   100,
		CostBudget:    "normal",
		RetryAttempts: 1,
	}
	outcome := runScrumLLMStageCheck(context.Background(), scrumLLMStage{
		Stage:    "review-risk-check",
		PresetID: "review",
		Agent:    "reviewer",
		Sprint:   2,
		Context:  "temperature zero review must return non-empty evidence",
	}, &preset)
	if !outcome.Success || outcome.TotalTokens != 13 || outcome.PresetID != "review" || outcome.Agent != "reviewer" {
		t.Fatalf("outcome = %+v, want successful review outcome with token usage", outcome)
	}
}

func TestRun_KubernetesRolloutE2ERequiresExplicitConfig(t *testing.T) {
	t.Setenv("ARUN_EVAL_KUBECONFIG", "")
	t.Setenv("ARUN_EVAL_KUBE_CONTEXT", "")
	t.Setenv("ARUN_EVAL_KUBE_NAMESPACE", "")
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
	for _, want := range []string{"ARUN_EVAL_KUBECONFIG", "ARUN_EVAL_KUBE_CONTEXT", "ARUN_EVAL_KUBE_NAMESPACE"} {
		if !strings.Contains(reasons, want) {
			t.Fatalf("failure reasons = %q, want %s", reasons, want)
		}
	}
}

func TestRun_RealLLMSmokeRequiresOptIn(t *testing.T) {
	t.Setenv("ARUN_EVAL_LIVE_LLM", "")
	report, err := Run(context.Background(), Options{
		WorkDir:                t.TempDir(),
		ScenarioIDs:            []string{"real-llm-orchestration-smoke"},
		IncludeRealLLMSmokeE2E: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Total != 1 || report.Passed != 0 || report.Failed != 1 {
		t.Fatalf("report = %+v, want one failing real LLM smoke scenario", report)
	}
	reasons := strings.Join(report.ScenarioRuns[0].FailureReasons, "\n")
	if !strings.Contains(reasons, "ARUN_EVAL_LIVE_LLM=true") {
		t.Fatalf("failure reasons = %q, want missing live LLM opt-in", reasons)
	}
}

func TestRun_LiteLLMPresetEvalsRequiresOptIn(t *testing.T) {
	t.Setenv("ARUN_EVAL_LLM_PRESETS", "")
	report, err := Run(context.Background(), Options{
		WorkDir:                   t.TempDir(),
		ScenarioIDs:               []string{"litellm-preset-matrix"},
		IncludeLiteLLMPresetEvals: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Total != 1 || report.Passed != 0 || report.Failed != 1 {
		t.Fatalf("report = %+v, want one failing LiteLLM preset scenario", report)
	}
	reasons := strings.Join(report.ScenarioRuns[0].FailureReasons, "\n")
	if !strings.Contains(reasons, "ARUN_EVAL_LLM_PRESETS=true") {
		t.Fatalf("failure reasons = %q, want missing LiteLLM preset opt-in", reasons)
	}
}

func TestRun_LiteLLMPresetEvalsReportsMatrix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s, want /chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"ready arun-preset-ok"}}],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}`))
	}))
	defer server.Close()
	t.Setenv("ARUN_EVAL_LLM_PRESETS", "true")
	t.Setenv("ARUN_EVAL_LLM_PRESET_MATRIX", `[{"id":"smoke","name":"Low-cost smoke","useCase":"smoke","provider":"litellm","baseUrl":"`+server.URL+`","model":"test-model","apiKeyEnv":"","timeout":"5s","temperature":0,"maxTokens":64,"retryAttempts":1,"tokenBudget":500,"costBudget":"low"}]`)
	report, err := Run(context.Background(), Options{
		WorkDir:                   t.TempDir(),
		ScenarioIDs:               []string{"litellm-preset-matrix"},
		IncludeLiteLLMPresetEvals: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Total != 1 || report.Failed != 0 || report.Passed != 1 {
		t.Fatalf("report = %+v, want one passing LiteLLM preset scenario", report)
	}
	result := report.ScenarioRuns[0]
	if result.Successes != 1 || result.Artifacts["successRate"] != "1.00" {
		t.Fatalf("result = %+v, want one successful preset with successRate 1.00", result)
	}
	if !strings.Contains(result.Artifacts["outcomes"], `"totalTokens":7`) || strings.Contains(result.Artifacts["outcomes"], "secret") {
		t.Fatalf("outcomes artifact = %s, want token usage without secrets", result.Artifacts["outcomes"])
	}
}

func TestCountingLLMClientCapsMaxTokens(t *testing.T) {
	inner := llm.NewMockLLMClient([]llm.ChatResponse{{
		Choices: []llm.Choice{{Message: llm.Message{Role: llm.RoleAssistant, Content: "{}"}}},
		Usage:   &llm.Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5},
	}})
	client := &countingLLMClient{inner: inner, maxTokens: 128}
	if _, err := client.Chat(context.Background(), llm.ChatRequest{Model: "test", MaxTokens: 4096}); err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if got := inner.Requests[0].MaxTokens; got != 128 {
		t.Fatalf("MaxTokens = %d, want 128", got)
	}
	if client.requests != 1 || client.successfulResponses != 1 || client.totalTokens != 5 {
		t.Fatalf("client counters = %+v", client)
	}
}

func TestSanitizeAuthE2EOutput(t *testing.T) {
	t.Setenv("ARUN_EVAL_AUTH_COOKIE", "arun_session=secret")
	got := sanitizeAuthE2EOutput("failed with arun_session=secret")
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

func TestSanitizeLiteLLMOutput(t *testing.T) {
	t.Setenv("LITELLM_API_KEY", "sk-secret")
	got := sanitizeLiteLLMOutput("request failed with sk-secret")
	if strings.Contains(got, "sk-secret") || !strings.Contains(got, "[redacted]") {
		t.Fatalf("sanitizeLiteLLMOutput() = %q", got)
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

func TestGitHubRepoAllowed(t *testing.T) {
	if !githubRepoAllowed("owner/repo", "owner/other, owner/repo") {
		t.Fatal("githubRepoAllowed() = false, want true")
	}
	if !githubRepoAllowed("owner/repo.git", "owner/repo") {
		t.Fatal("githubRepoAllowed() did not normalize .git suffix")
	}
	if githubRepoAllowed("owner/repo", "owner/repo-extra") {
		t.Fatal("githubRepoAllowed() accepted partial match")
	}
}

func TestScrumGitHubCleanupMode(t *testing.T) {
	t.Setenv("ARUN_EVAL_SCRUM_GITHUB_CLEANUP", "")
	if got := scrumGitHubCleanupMode(); got != "close" {
		t.Fatalf("scrumGitHubCleanupMode() = %q, want close", got)
	}
	t.Setenv("ARUN_EVAL_SCRUM_GITHUB_CLEANUP", "keep")
	if got := scrumGitHubCleanupMode(); got != "keep" {
		t.Fatalf("scrumGitHubCleanupMode() = %q, want keep", got)
	}
	t.Setenv("ARUN_EVAL_SCRUM_GITHUB_CLEANUP", "delete")
	if got := scrumGitHubCleanupMode(); got != "" {
		t.Fatalf("scrumGitHubCleanupMode() = %q, want empty invalid mode", got)
	}
}

func TestScrumIssueIDFromTitle(t *testing.T) {
	got := scrumIssueIDFromTitle("[ARUN Eval][20260702T060704] AOS-106 Investigate flaky live LLM smoke")
	if got != "AOS-106" {
		t.Fatalf("scrumIssueIDFromTitle() = %q, want AOS-106", got)
	}
	if got := scrumIssueIDFromTitle("[ARUN Eval] missing id"); got != "" {
		t.Fatalf("scrumIssueIDFromTitle() = %q, want empty", got)
	}
}

func TestScrumCleanupSucceeded(t *testing.T) {
	closed := []scrumGitHubArtifact{{CleanupStatus: "closed", FinalState: "closed"}, {CleanupStatus: "already-closed", FinalState: "closed"}}
	if !scrumCleanupSucceeded(closed, "close") {
		t.Fatal("scrumCleanupSucceeded(close) = false, want true")
	}
	kept := []scrumGitHubArtifact{{CleanupStatus: "kept", FinalState: "open"}}
	if !scrumCleanupSucceeded(kept, "keep") {
		t.Fatal("scrumCleanupSucceeded(keep) = false, want true")
	}
	failed := []scrumGitHubArtifact{{CleanupStatus: "failed", FinalState: "open"}}
	if scrumCleanupSucceeded(failed, "close") {
		t.Fatal("scrumCleanupSucceeded(failed close) = true, want false")
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
	t.Setenv("ARUN_EVAL_AUTH_E2E_SCRIPT", script)
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
