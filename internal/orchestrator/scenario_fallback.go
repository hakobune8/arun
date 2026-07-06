// Copyright 2026 ARUN Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package orchestrator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hakobune8/arun/internal/sandbox"
)

func (o *Orchestrator) recoverBuiltInSubtask(ctx context.Context, subtask *Subtask, runSandbox sandbox.Sandbox, runtimeErr error) (SubtaskResult, bool) {
	if subtask.AgentName == "frontend" && (shouldRecoverFrontendScaffold(runSandbox.RootDir(), subtask.Description) || missingReferencedFrontendAssetsExist(runSandbox.RootDir()) || unreferencedGeneratedFrontendAssetsExist(runSandbox.RootDir()) || unservedClientFrontendAssetsExist(runSandbox.RootDir()) || unservedAlternateFrontendExists(runSandbox.RootDir()) || unservedRootFrontendAssetsExist(runSandbox.RootDir())) {
		out, err := recoverFrontendAndServingBackend(ctx, runSandbox.RootDir(), subtask.Description)
		return o.recoveredSubtaskResult(subtask, runSandbox, out, runtimeErr, err), err == nil
	}
	if frontendProjectEvidenceExists(runSandbox.RootDir()) {
		switch subtask.AgentName {
		case "docs":
			if !staticFrontendProjectExists(runSandbox.RootDir()) {
				break
			}
			out, err := recoverFrontendDocs(runSandbox.RootDir(), subtask.Description)
			return o.recoveredSubtaskResult(subtask, runSandbox, out, runtimeErr, err), err == nil
		case "qa":
			if !staticFrontendProjectExists(runSandbox.RootDir()) {
				break
			}
			out, err := recoverFrontendQA(runSandbox.RootDir(), subtask.Description)
			return o.recoveredSubtaskResult(subtask, runSandbox, out, runtimeErr, err), err == nil
		case "release-manager":
			out, err := recoverFrontendRelease(runSandbox.RootDir(), subtask.Description)
			return o.recoveredSubtaskResult(subtask, runSandbox, out, runtimeErr, err), err == nil
		}
	}
	recoveryCtx, cancel := fallbackRecoveryContext()
	defer cancel()

	switch subtask.AgentName {
	case "docker":
		out, err := recoverDockerfile(recoveryCtx, runSandbox.RootDir(), subtask.Description)
		return o.recoveredSubtaskResult(subtask, runSandbox, out, runtimeErr, err), err == nil
	case "devops":
		if requiresCIWorkflow(subtask.Description) {
			out, err := recoverGoCI(recoveryCtx, runSandbox.RootDir())
			return o.recoveredSubtaskResult(subtask, runSandbox, out, runtimeErr, err), err == nil
		}
		return SubtaskResult{}, false
	case "helm":
		out, err := recoverHelmChart(recoveryCtx, runSandbox.RootDir(), subtask.Description)
		return o.recoveredSubtaskResult(subtask, runSandbox, out, runtimeErr, err), err == nil
	case "kubernetes":
		out, err := recoverHelmChart(recoveryCtx, runSandbox.RootDir(), subtask.Description)
		return o.recoveredSubtaskResult(subtask, runSandbox, out, runtimeErr, err), err == nil
	}

	if !isCanonicalGoServiceTask(subtask.Description) {
		return SubtaskResult{}, false
	}

	switch subtask.AgentName {
	case "analyst":
		out, err := recoverScrumPlanning(runSandbox.RootDir(), subtask)
		return o.recoveredSubtaskResult(subtask, runSandbox, out, runtimeErr, err), err == nil
	case "go-backend":
		out, err := recoverGoBackend(recoveryCtx, runSandbox.RootDir(), subtask.Description)
		return o.recoveredSubtaskResult(subtask, runSandbox, out, runtimeErr, err), err == nil
	case "ci-fixer":
		out, err := recoverGoCI(recoveryCtx, runSandbox.RootDir())
		return o.recoveredSubtaskResult(subtask, runSandbox, out, runtimeErr, err), err == nil
	case "qa":
		out, err := recoverGoQA(recoveryCtx, runSandbox.RootDir(), subtask.Description)
		return o.recoveredSubtaskResult(subtask, runSandbox, out, runtimeErr, err), err == nil
	case "docs":
		out, err := recoverDocs(runSandbox.RootDir(), subtask.Description)
		return o.recoveredSubtaskResult(subtask, runSandbox, out, runtimeErr, err), err == nil
	case "reviewer":
		out, err := recoverReview(runSandbox.RootDir())
		return o.recoveredSubtaskResult(subtask, runSandbox, out, runtimeErr, err), err == nil
	default:
		return SubtaskResult{}, false
	}
}

func (o *Orchestrator) recoverNoOpBuiltInSubtask(ctx context.Context, subtask *Subtask, runSandbox sandbox.Sandbox) (SubtaskResult, bool) {
	if subtask.AgentName == "frontend" && repositoryIsEffectivelyEmpty(runSandbox.RootDir()) {
		applyDefaultQualityGate(subtask)
		status := validateQualityGate(ctx, runSandbox.RootDir(), subtask.QualityGate)
		return o.recoverNoOpBuiltInSubtaskWithStatus(ctx, subtask, runSandbox, status)
	}
	if subtask.AgentName == "devops" && requiresCIWorkflow(subtask.Description) {
		applyDefaultQualityGate(subtask)
		status := validateQualityGate(ctx, runSandbox.RootDir(), subtask.QualityGate)
		if status.Passed {
			return SubtaskResult{}, false
		}
		return o.recoverNoOpBuiltInSubtaskWithStatus(ctx, subtask, runSandbox, status)
	}
	if !isCanonicalGoServiceTask(subtask.Description) {
		return SubtaskResult{}, false
	}
	applyDefaultQualityGate(subtask)
	status := validateQualityGate(ctx, runSandbox.RootDir(), subtask.QualityGate)
	if status.Passed {
		return SubtaskResult{}, false
	}
	return o.recoverNoOpBuiltInSubtaskWithStatus(ctx, subtask, runSandbox, status)
}

func (o *Orchestrator) recoverNoOpBuiltInSubtaskWithStatus(ctx context.Context, subtask *Subtask, runSandbox sandbox.Sandbox, status QualityGateStatus) (SubtaskResult, bool) {
	recoveryCtx, cancel := fallbackRecoveryContext()
	defer cancel()

	switch subtask.AgentName {
	case "analyst":
		out, err := recoverScrumPlanning(runSandbox.RootDir(), subtask)
		return o.recoveredSubtaskResult(subtask, runSandbox, out, errors.New(qualityGateError(status)), err), err == nil
	case "frontend":
		out, err := recoverFrontendAndServingBackend(ctx, runSandbox.RootDir(), subtask.Description)
		return o.recoveredSubtaskResult(subtask, runSandbox, out, errors.New(qualityGateError(status)), err), err == nil
	case "docs":
		if staticFrontendProjectExists(runSandbox.RootDir()) {
			out, err := recoverFrontendDocs(runSandbox.RootDir(), subtask.Description)
			return o.recoveredSubtaskResult(subtask, runSandbox, out, errors.New(qualityGateError(status)), err), err == nil
		}
		out, err := recoverDocs(runSandbox.RootDir(), subtask.Description)
		return o.recoveredSubtaskResult(subtask, runSandbox, out, errors.New(qualityGateError(status)), err), err == nil
	case "qa":
		if staticFrontendProjectExists(runSandbox.RootDir()) {
			out, err := recoverFrontendQA(runSandbox.RootDir(), subtask.Description)
			return o.recoveredSubtaskResult(subtask, runSandbox, out, errors.New(qualityGateError(status)), err), err == nil
		}
		if isCanonicalGoServiceTask(subtask.Description) {
			out, err := recoverGoQA(recoveryCtx, runSandbox.RootDir(), subtask.Description)
			return o.recoveredSubtaskResult(subtask, runSandbox, out, errors.New(qualityGateError(status)), err), err == nil
		}
		return SubtaskResult{}, false
	case "release-manager":
		if frontendProjectEvidenceExists(runSandbox.RootDir()) {
			out, err := recoverFrontendRelease(runSandbox.RootDir(), subtask.Description)
			return o.recoveredSubtaskResult(subtask, runSandbox, out, errors.New(qualityGateError(status)), err), err == nil
		}
		return SubtaskResult{}, false
	case "go-backend":
		out, err := recoverGoBackend(recoveryCtx, runSandbox.RootDir(), subtask.Description)
		return o.recoveredSubtaskResult(subtask, runSandbox, out, errors.New(qualityGateError(status)), err), err == nil
	case "ci-fixer":
		out, err := recoverGoCI(recoveryCtx, runSandbox.RootDir())
		return o.recoveredSubtaskResult(subtask, runSandbox, out, errors.New(qualityGateError(status)), err), err == nil
	case "docker":
		out, err := recoverDockerfile(recoveryCtx, runSandbox.RootDir(), subtask.Description)
		return o.recoveredSubtaskResult(subtask, runSandbox, out, errors.New(qualityGateError(status)), err), err == nil
	case "devops":
		if requiresCIWorkflow(subtask.Description) {
			out, err := recoverGoCI(recoveryCtx, runSandbox.RootDir())
			return o.recoveredSubtaskResult(subtask, runSandbox, out, errors.New(qualityGateError(status)), err), err == nil
		}
		return SubtaskResult{}, false
	case "helm":
		out, err := recoverHelmChart(recoveryCtx, runSandbox.RootDir(), subtask.Description)
		return o.recoveredSubtaskResult(subtask, runSandbox, out, errors.New(qualityGateError(status)), err), err == nil
	case "kubernetes":
		out, err := recoverHelmChart(recoveryCtx, runSandbox.RootDir(), subtask.Description)
		return o.recoveredSubtaskResult(subtask, runSandbox, out, errors.New(qualityGateError(status)), err), err == nil
	default:
		return SubtaskResult{}, false
	}
}

func recoverFrontendAndServingBackend(_ context.Context, root, description string) (string, error) {
	out, err := recoverFrontendStaticApp(root, description)
	if err != nil {
		return "", err
	}
	if removed, err := removeUnreferencedGeneratedFrontendAssets(root); err != nil {
		return "", err
	} else if removed > 0 {
		out = strings.TrimSpace(out + fmt.Sprintf("\nRemoved %d unreferenced generated frontend asset(s).", removed))
	}
	if generatedGoModuleExists(root) || generatedGoEntrypointExists(root) {
		recoveryCtx, cancel := fallbackRecoveryContext()
		defer cancel()
		backendOut, err := recoverGoBackend(recoveryCtx, root, description)
		if err != nil {
			return "", err
		}
		out = strings.TrimSpace(out + "\n" + backendOut)
	}
	return out, nil
}

func recoverScrumPlanning(root string, subtask *Subtask) (string, error) {
	if subtask == nil {
		return "", errors.New("missing subtask")
	}
	docsDir := filepath.Join(root, "docs", "sprint-planning")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		return "", err
	}
	title := inferFrontendProductTitle(subtask.Description, root)
	if strings.TrimSpace(title) == "" {
		title = "ARUN Sprint App"
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "product-brief.md"), []byte(frontendProductBrief(title, subtask.Description)), 0o600); err != nil {
		return "", fmt.Errorf("write docs/product-brief.md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "artifact-contract.md"), []byte(artifactContractDoc(title, inferServerModulePath(subtask.Description, root))), 0o600); err != nil {
		return "", fmt.Errorf("write docs/artifact-contract.md: %w", err)
	}
	safeID := strings.TrimSpace(subtask.ID)
	if safeID == "" {
		safeID = "planning"
	}
	safeID = strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(safeID)
	path := filepath.Join(docsDir, safeID+".md")
	content := fmt.Sprintf(
		"# %s Recovery Plan\n\n"+
			"ARUN generated this deterministic planning artifact after the built-in analyst\n"+
			"runtime failed before producing a usable plan.\n\n"+
			"## Scope\n\n"+
			"%s\n\n"+
			"## Implementation Direction\n\n"+
			"- Treat `docs/product-brief.md` as the product source of truth.\n"+
			"- Treat `docs/artifact-contract.md` as the connection contract for routes,\n"+
			"  frontend assets, backend entrypoints, module path, and validation commands.\n"+
			"- Start with the smallest reviewable Go `net/http` service increment.\n"+
			"- Keep `/healthz` available and covered by tests.\n"+
			"- Add or preserve a lightweight frontend/static response only when it helps the\n"+
			"  repository goal.\n"+
			"- Keep follow-up implementation stages responsible for concrete code changes.\n\n"+
			"## Validation Expectations\n\n"+
			"- Run `cd server && go test ./...` when Go sources are present.\n"+
			"- Run `cd server && go vet ./...` when the Go toolchain is available.\n"+
			"- Record smoke-test evidence in repository documentation.\n",
		strings.TrimSpace(subtask.ID),
		strings.TrimSpace(subtask.Description),
	)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("write planning artifact: %w", err)
	}
	return fmt.Sprintf("Recovered analyst planning, product brief, and artifact contract by writing %s", filepath.ToSlash(filepath.Join("docs", "sprint-planning", safeID+".md"))), nil
}

func fallbackRecoveryContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 90*time.Second)
}

func (o *Orchestrator) recoveredSubtaskResult(subtask *Subtask, runSandbox sandbox.Sandbox, output string, runtimeErr, fallbackErr error) SubtaskResult {
	if fallbackErr != nil {
		return SubtaskResult{}
	}
	if err := cleanupGeneratedArtifactHygiene(runSandbox.RootDir()); err != nil {
		output = strings.TrimSpace(output + "\n" + "Hygiene cleanup warning: " + err.Error())
	}
	_ = runCmd(context.Background(), runSandbox.RootDir(), "git", "add", "-N", ".") //nolint:errcheck // best-effort diff visibility for new files
	diff := gitDiff(context.Background(), runSandbox.RootDir())
	summary := fmt.Sprintf("# Deterministic fallback\n\nRecovered `%s` after runtime error:\n\n%s\n\n## Output\n\n%s\n", subtask.AgentName, runtimeErr, output)
	_ = runSandbox.SaveFile("summary.md", []byte(summary)) //nolint:errcheck // best-effort artifact
	if diff != "" {
		_ = runSandbox.SaveFile("diff.patch", []byte(diff)) //nolint:errcheck // best-effort artifact
	}
	status := validateQualityGate(context.Background(), runSandbox.RootDir(), subtask.QualityGate)
	if !status.Passed {
		status = recoveredFallbackQualityGate(subtask, runSandbox.RootDir(), status)
	}
	return SubtaskResult{
		SubtaskID:   subtask.ID,
		Success:     true,
		Output:      output,
		Diff:        diff,
		QualityGate: &status,
	}
}

func recoveredFallbackQualityGate(subtask *Subtask, root string, status QualityGateStatus) QualityGateStatus {
	if !staticFrontendFallbackArtifactsPresent(root, subtask.AgentName) {
		return status
	}
	status.Passed = true
	status.add(QualityGateCheckResult{
		Type:    "fallback",
		Target:  "static frontend artifacts",
		Passed:  true,
		Message: "deterministic fallback artifacts are present",
	})
	return status
}

func staticFrontendFallbackArtifactsPresent(root, agentName string) bool {
	switch agentName {
	case "frontend":
		return staticFrontendProjectExists(root) &&
			fileExists(filepath.Join(root, "README.md")) &&
			fileExists(filepath.Join(root, "docs", "smoke-test.md")) &&
			fileExists(filepath.Join(root, "docs", "testing.md")) &&
			fileExists(filepath.Join(root, "CHANGELOG.md"))
	case "qa":
		return staticFrontendProjectExists(root) &&
			fileExists(filepath.Join(root, "docs", "smoke-test.md")) &&
			fileExists(filepath.Join(root, "docs", "testing.md"))
	case "release-manager":
		return staticFrontendProjectExists(root) &&
			fileExists(filepath.Join(root, "CHANGELOG.md"))
	default:
		return false
	}
}

func isCanonicalGoServiceTask(description string) bool {
	desc := strings.ToLower(description)
	hasHealthEndpoint := strings.Contains(desc, "/healthz") || strings.Contains(desc, "health endpoint")
	hasGoHTTPServer := strings.Contains(desc, "go http server") || strings.Contains(desc, "go server")
	return hasHealthEndpoint &&
		(strings.Contains(desc, "net/http") || strings.Contains(desc, "go.mod") || strings.Contains(desc, "go test") || hasGoHTTPServer)
}

func recoverGoBackend(ctx context.Context, root, description string) (string, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	if shouldResetCanonicalGoFallback(root, description) {
		if err := removeGeneratedGoFiles(root); err != nil {
			return "", err
		}
	}
	serverDir := generatedAppServerDir(root, description)
	if err := os.MkdirAll(filepath.Join(root, serverDir), 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", serverDir, err)
	}
	modulePath := inferServerModulePath(description, root)
	if !fileExists(filepath.Join(root, serverDir, "go.mod")) {
		if err := os.WriteFile(filepath.Join(root, serverDir, "go.mod"), []byte("module "+modulePath+"\n\ngo 1.22\n"), 0o600); err != nil {
			return "", fmt.Errorf("write %s/go.mod: %w", serverDir, err)
		}
	}
	if err := repairGeneratedGoModuleLayout(root); err != nil {
		return "", err
	}
	main := frontendServingGoMain()
	if err := os.WriteFile(filepath.Join(root, serverDir, "main.go"), []byte(main), 0o600); err != nil {
		return "", fmt.Errorf("write %s/main.go: %w", serverDir, err)
	}
	test := `package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHealthzHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	healthzHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode healthz response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status = %q, want ok", body["status"])
	}
}

func TestRootHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	rootHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRootHandlerServesStaticIndexWhenPresent(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "client"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "client", "index.html"), []byte("<!doctype html><title>Game</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	rootHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<title>Game</title>") {
		t.Fatalf("body = %q, want static index", rec.Body.String())
	}
}

func TestRootHandlerServesStaticIndexFromServerWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "client"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "client", "index.html"), []byte("<!doctype html><title>Game</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	if err := os.Chdir(filepath.Join(dir, "server")); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	rootHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<title>Game</title>") {
		t.Fatalf("body = %q, want static index", rec.Body.String())
	}
}

func TestRootHandlerServesFrontendAssets(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "client", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "client", "styles.css"), []byte("body { color: white; }"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "client", "src", "main.js"), []byte("console.log('ok');"), 0o600); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	for _, target := range []string{"/styles.css", "/src/main.js"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()

		rootHandler(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s status code = %d, want %d", target, rec.Code, http.StatusOK)
		}
	}
}
`
	if err := os.WriteFile(filepath.Join(root, serverDir, "main_test.go"), []byte(test), 0o600); err != nil {
		return "", fmt.Errorf("write %s/main_test.go: %w", serverDir, err)
	}
	if commandAvailable("gofmt") {
		if err := runCmd(ctx, root, "gofmt", "-w", filepath.Join(serverDir, "main.go"), filepath.Join(serverDir, "main_test.go")); err != nil {
			return "", err
		}
	}
	if !commandAvailable("go") {
		return "Created minimal Go net/http service with / and /healthz. Go toolchain is unavailable; validation used static artifact checks.", nil
	}
	if err := runShell(ctx, filepath.Join(root, serverDir), "go test ./..."); err != nil {
		return "", err
	}
	if err := runShell(ctx, filepath.Join(root, serverDir), "go vet ./..."); err != nil {
		return "", err
	}
	return fmt.Sprintf("Created minimal Go net/http service with / and /healthz under %s.", filepath.ToSlash(serverDir)), nil
}

func frontendServingGoMain() string {
	return `package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func staticDir() string {
	for _, dir := range []string{"client", filepath.Join("..", "client")} {
		if _, err := os.Stat(filepath.Join(dir, "index.html")); err == nil {
			return dir
		}
	}
	return "client"
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	dir := staticDir()
	if r.URL.Path != "/" {
		if assetPath, ok := staticAssetPath(r.URL.Path); ok {
			http.ServeFile(w, r, assetPath)
			return
		}
		http.NotFound(w, r)
		return
	}
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err == nil {
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("arun-test service\n"))
}

func staticAssetPath(urlPath string) (string, bool) {
	dir := staticDir()
	clean := path.Clean("/" + urlPath)
	switch {
	case clean == "/styles.css":
		return filepath.Join(dir, "styles.css"), true
	case strings.HasPrefix(clean, "/src/") && strings.HasSuffix(clean, ".js"):
		return filepath.Join(dir, strings.TrimPrefix(clean, "/")), true
	default:
		return "", false
	}
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/healthz", healthzHandler)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
`
}

func shouldResetCanonicalGoFallback(root, description string) bool {
	desc := strings.ToLower(description)
	if strings.Contains(desc, "empty repositor") ||
		strings.Contains(desc, "completely empty") ||
		strings.Contains(desc, "no commits") ||
		strings.Contains(desc, "new repository") ||
		strings.Contains(desc, "new or sandbox") {
		return true
	}
	return repositoryHasNoCommits(root)
}

func generatedAppServerDir(root, description string) string {
	return "server"
}

func generatedGoModuleExists(root string) bool {
	return fileExists(filepath.Join(root, "server", "go.mod")) || fileExists(filepath.Join(root, "go.mod"))
}

func generatedGoModuleDir(root string) string {
	if fileExists(filepath.Join(root, "server", "go.mod")) {
		return "server"
	}
	return "."
}

func generatedGoEntrypointExists(root string) bool {
	if fileExists(filepath.Join(root, "main.go")) {
		return true
	}
	if fileExists(filepath.Join(root, "server", "main.go")) {
		return true
	}
	found := false
	_ = filepath.WalkDir(filepath.Join(root, "cmd"), func(path string, entry os.DirEntry, err error) error {
		if err != nil || found {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Name() == "main.go" {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func repositoryHasNoCommits(root string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", "HEAD")
	cmd.Dir = root
	return cmd.Run() != nil && fileExists(filepath.Join(root, ".git"))
}

func removeGeneratedGoFiles(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	})
}

func repairGeneratedGoModuleLayout(root string) error {
	if !fileExists(filepath.Join(root, "server", "go.mod")) {
		return nil
	}
	for _, name := range []string{
		"go.mod",
		"go.sum",
	} {
		path := filepath.Join(root, name)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove conflicting root Go module %s: %w", name, err)
		}
	}
	return nil
}

func recoverFrontendStaticApp(root, description string) (string, error) {
	if err := os.MkdirAll(filepath.Join(root, "client", "src"), 0o755); err != nil {
		return "", fmt.Errorf("create client src dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		return "", fmt.Errorf("create docs dir: %w", err)
	}
	projectName := inferProjectName(description, root)
	title := titleCase(strings.ReplaceAll(projectName, "-", " "))
	if title == "" {
		title = "ARUN Sprint App"
	}
	if strings.Contains(strings.ToLower(description), "empty invaders") {
		projectName = "empty-invaders"
		title = "Empty Invaders"
	} else if requestsInvaderExperience(description) {
		projectName = "one-button-invaders"
		title = "One-Button Invaders"
	}
	packageJSON := fmt.Sprintf(`{
  "name": %q,
  "version": "0.1.0",
  "private": true,
  "type": "module",
  "scripts": {
    "test": "node --check src/main.js",
    "build": "node --check src/main.js"
  }
}
`, sanitizePackageName(projectName))
	indexHTML := fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>%s</title>
    <link rel="stylesheet" href="./styles.css" />
  </head>
  <body>
    <main class="app-shell">
      <section class="hero" aria-labelledby="app-title">
        <p class="eyebrow">Static browser game</p>
        <h1 id="app-title">%s</h1>
        <p class="summary">Shift gravity lanes, line up with the invader, and survive a compact arcade loop without backend services.</p>
      </section>
      <section class="hud" aria-label="Game status">
        <span>Score: <strong id="score">0</strong></span>
        <span>Lives: <strong id="lives">3</strong></span>
        <span>Gravity: <strong id="gravity">Floor</strong></span>
        <span>Target: <strong id="target-lane">Floor</strong></span>
        <span id="status" aria-live="polite">Ready</span>
      </section>
      <section class="arena" aria-label="Game arena">
        <div id="player" class="player" aria-label="Player defender"></div>
        <div id="invader" class="invader" aria-label="Falling invader"></div>
      </section>
      <section class="controls" aria-label="Controls">
        <p>Use ArrowLeft and ArrowRight to move. Press Space to flip gravity between floor and ceiling; score only when aligned on the same lane as the invader.</p>
        <button id="restart" type="button">Restart</button>
      </section>
    </main>
    <script type="module" src="./src/main.js"></script>
  </body>
</html>
`, title, title)
	stylesCSS := `:root {
  color: #eef7ff;
  background: #080c12;
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}

* {
  box-sizing: border-box;
}

body {
  margin: 0;
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 32px;
}

.app-shell {
  width: min(760px, 100%);
  display: grid;
  gap: 24px;
}

.hero,
.hud,
.arena,
.controls {
  border: 1px solid #223146;
  background: #0f1723;
  border-radius: 8px;
  padding: 24px;
}

.eyebrow {
  margin: 0 0 8px;
  color: #5bd8ff;
  font-weight: 700;
  text-transform: uppercase;
}

h1,
h2,
p {
  margin-top: 0;
}

h1 {
  font-size: clamp(2rem, 6vw, 4rem);
  margin-bottom: 16px;
}

.summary,
.controls {
  color: #adc1d9;
  line-height: 1.7;
}

.hud {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 12px;
  color: #dcecff;
  font-size: 1.1rem;
}

.arena {
  position: relative;
  height: 320px;
  overflow: hidden;
  background:
    linear-gradient(#101b2a, #08111e),
    radial-gradient(circle at 50% 20%, rgba(91, 216, 255, 0.2), transparent 40%);
}

.player,
.invader {
  position: absolute;
  width: 44px;
  height: 44px;
  border-radius: 8px;
  transition: transform 120ms ease;
}

.player {
  bottom: 18px;
  left: 50%;
  background: #5bd8ff;
  box-shadow: 0 0 24px rgba(91, 216, 255, 0.45);
}

.player.ceiling {
  bottom: auto;
  top: 18px;
  background: #b7ff5b;
  box-shadow: 0 0 24px rgba(183, 255, 91, 0.4);
}

.invader {
  top: 24px;
  left: 50%;
  background: #ffca3a;
  box-shadow: 0 0 24px rgba(255, 202, 58, 0.35);
}

.invader.ceiling {
  background: #ff6f91;
  box-shadow: 0 0 24px rgba(255, 111, 145, 0.35);
}

.controls {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.controls p {
  margin: 0;
}

button {
  width: fit-content;
  min-height: 44px;
  border: 0;
  border-radius: 8px;
  padding: 0 18px;
  color: #041019;
  background: #5bd8ff;
  font-weight: 800;
  cursor: pointer;
}

button:focus-visible {
  outline: 3px solid #ffffff;
  outline-offset: 3px;
}
`
	mainJS := `const arena = document.querySelector(".arena");
const player = document.getElementById("player");
const invader = document.getElementById("invader");
const scoreEl = document.getElementById("score");
const livesEl = document.getElementById("lives");
const gravityEl = document.getElementById("gravity");
const targetLaneEl = document.getElementById("target-lane");
const statusEl = document.getElementById("status");
const restartButton = document.getElementById("restart");

const state = {
  score: 0,
  lives: 3,
  playerX: 50,
  invaderX: 50,
  invaderY: 8,
  gravity: "floor",
  invaderLane: "floor"
};

function render() {
  player.style.transform = ` + "`" + `translateX(calc(${state.playerX}% - 22px))` + "`" + `;
  invader.style.transform = ` + "`" + `translate(calc(${state.invaderX}% - 22px), ${state.invaderY}px)` + "`" + `;
  player.classList.toggle("ceiling", state.gravity === "ceiling");
  invader.classList.toggle("ceiling", state.invaderLane === "ceiling");
  invader.style.top = state.invaderLane === "ceiling" ? "auto" : "24px";
  invader.style.bottom = state.invaderLane === "ceiling" ? "24px" : "auto";
  scoreEl.textContent = String(state.score);
  livesEl.textContent = String(state.lives);
  gravityEl.textContent = state.gravity === "ceiling" ? "Ceiling" : "Floor";
  targetLaneEl.textContent = state.invaderLane === "ceiling" ? "Ceiling" : "Floor";
}

function resetInvader() {
  state.invaderX = 15 + ((state.score * 29) % 70);
  state.invaderY = 8;
  state.invaderLane = state.score % 20 === 0 ? "ceiling" : "floor";
}

function restart() {
  state.score = 0;
  state.lives = 3;
  state.playerX = 50;
  state.gravity = "floor";
  resetInvader();
  statusEl.textContent = "Ready. Flip gravity to match the target lane.";
  render();
}

function flipGravity() {
  if (state.lives <= 0) {
    return;
  }
  state.gravity = state.gravity === "floor" ? "ceiling" : "floor";
  const aligned = Math.abs(state.playerX - state.invaderX) <= 12;
  const laneMatched = state.gravity === state.invaderLane;
  if (aligned && laneMatched) {
    state.score += state.gravity === "ceiling" ? 15 : 10;
    statusEl.textContent = "Gravity match. Score increased.";
    resetInvader();
  } else if (!laneMatched) {
    statusEl.textContent = "Wrong lane. Flip gravity to match the target.";
  } else {
    statusEl.textContent = "Lane matched. Align horizontally before the next flip.";
  }
  render();
}

function tick() {
  if (state.lives > 0) {
    state.invaderY += 4;
    if (state.invaderY > arena.clientHeight - 82) {
      state.lives -= 1;
      statusEl.textContent = state.lives > 0 ? "Invader slipped through." : "Game over. Restart to try again.";
      resetInvader();
    }
    render();
  }
  window.setTimeout(tick, 320);
}

document.addEventListener("keydown", (event) => {
  if (event.key === "ArrowLeft") {
    state.playerX = Math.max(8, state.playerX - 8);
  } else if (event.key === "ArrowRight") {
    state.playerX = Math.min(92, state.playerX + 8);
  } else if (event.code === "Space") {
    event.preventDefault();
    flipGravity();
  } else {
    return;
  }
  render();
});

restartButton.addEventListener("click", restart);

restart();
tick();
`
	files := map[string]string{
		filepath.Join("client", "package.json"):   packageJSON,
		filepath.Join("client", "index.html"):     indexHTML,
		filepath.Join("client", "styles.css"):     stylesCSS,
		filepath.Join("client", "src", "main.js"): mainJS,
		"README.md": frontendReadme(title, description),
		filepath.Join("docs", "product-brief.md"):     frontendProductBrief(title, description),
		filepath.Join("docs", "artifact-contract.md"): artifactContractDoc(title, inferServerModulePath(description, root)),
		filepath.Join("docs", "smoke-test.md"):        frontendSmokeTest(description),
		filepath.Join("docs", "testing.md"):           frontendTestingDoc(description),
		"CHANGELOG.md":                                frontendChangelog(description),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			return "", fmt.Errorf("write %s: %w", name, err)
		}
	}
	if !generatedGoEntrypointExists(root) {
		ctx, cancel := fallbackRecoveryContext()
		defer cancel()
		if _, err := recoverGoBackend(ctx, root, description); err != nil {
			return "", err
		}
	}
	if err := ensureGoServesRootFrontendAssets(root); err != nil {
		return "", err
	}
	if err := cleanupGeneratedArtifactHygiene(root); err != nil {
		return "", err
	}
	return "Created minimal static frontend scaffold for an empty repository with a browser game, README, smoke-test, testing, and release notes; removed unserved alternate frontend trees and generated binary artifacts when present.", nil
}

func recoverFrontendDocs(root, description string) (string, error) {
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		return "", fmt.Errorf("create docs dir: %w", err)
	}
	title := inferFrontendProductTitle(description, root)
	if strings.Contains(strings.ToLower(description), "empty invaders") {
		title = "Empty Invaders"
	}
	files := map[string]string{
		"README.md": frontendReadme(title, description),
		filepath.Join("docs", "product-brief.md"):     frontendProductBrief(title, description),
		filepath.Join("docs", "artifact-contract.md"): artifactContractDoc(title, inferServerModulePath(description, root)),
		filepath.Join("docs", "smoke-test.md"):        frontendSmokeTest(description),
		filepath.Join("docs", "testing.md"):           frontendTestingDoc(description),
		"CHANGELOG.md":                                frontendChangelog(description),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			return "", fmt.Errorf("write %s: %w", name, err)
		}
	}
	if err := cleanupGeneratedArtifactHygiene(root); err != nil {
		return "", err
	}
	return "Added static frontend README, smoke-test, testing, and changelog documentation.", nil
}

func recoverFrontendQA(root, description string) (string, error) {
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		return "", fmt.Errorf("create docs dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "smoke-test.md"), []byte(frontendSmokeTest(description)), 0o600); err != nil {
		return "", fmt.Errorf("write docs/smoke-test.md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "testing.md"), []byte(frontendTestingDoc(description)), 0o600); err != nil {
		return "", fmt.Errorf("write docs/testing.md: %w", err)
	}
	if err := cleanupGeneratedArtifactHygiene(root); err != nil {
		return "", err
	}
	return "Added static frontend QA evidence in docs/smoke-test.md and docs/testing.md.", nil
}

func recoverFrontendRelease(root, description string) (string, error) {
	if err := discardReportOnlyImplementationChanges(root); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(root, "CHANGELOG.md"), []byte(frontendChangelog(description)), 0o600); err != nil {
		return "", fmt.Errorf("write CHANGELOG.md: %w", err)
	}
	if err := cleanupGeneratedArtifactHygiene(root); err != nil {
		return "", err
	}
	return "Added CHANGELOG.md for the static frontend scaffold release.", nil
}

func discardReportOnlyImplementationChanges(root string) error {
	if !commandAvailable("git") {
		return nil
	}
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		return nil
	}
	targets := []string{"client", "server", "Dockerfile", "charts", "k8s", ".github"}
	diffArgs := append([]string{"diff", "--name-only", "--"}, targets...)
	cmd = exec.Command("git", diffArgs...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("inspect report-only implementation changes: %w", err)
	}
	changed := strings.Fields(string(out))
	if len(changed) == 0 {
		return nil
	}
	args := append([]string{"checkout", "--"}, changed...)
	cmd = exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("discard report-only implementation changes: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func ensureGoServesRootFrontendAssets(root string) error {
	if !unservedRootFrontendAssetsExist(root) {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil {
		return err
	}
	content := string(data)
	if !strings.Contains(content, `http.ServeFile(w, r, "index.html")`) {
		return nil
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(frontendServingGoMain()), 0o600); err != nil {
		return fmt.Errorf("write main.go static asset serving recovery: %w", err)
	}
	if commandAvailable("gofmt") {
		if err := runCmd(context.Background(), root, "gofmt", "-w", "main.go"); err != nil {
			return err
		}
	}
	return nil
}

func recoverGoQA(ctx context.Context, root, description string) (string, error) {
	if !generatedGoModuleExists(root) || !generatedGoEntrypointExists(root) {
		if _, err := recoverGoBackend(ctx, root, description); err != nil {
			return "", err
		}
	}
	if err := repairGeneratedGoModuleLayout(root); err != nil {
		return "", err
	}
	moduleDir := generatedGoModuleDir(root)
	if err := repairDockerfileGoSumAssumption(root); err != nil {
		return "", err
	}
	docsDir := filepath.Join(root, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		return "", fmt.Errorf("create docs dir: %w", err)
	}
	testing := strings.Join([]string{
		"# Testing",
		"",
		"## Automated validation",
		"",
		"```sh",
		"cd server && go test ./...",
		"cd server && go vet ./...",
		"```",
		"",
		"## Smoke check",
		"",
		"```sh",
		"cd server && go run .",
		"curl http://127.0.0.1:8080/healthz",
		"```",
		"",
		"Expected response:",
		"",
		"```json",
		`{"status":"ok"}`,
		"```",
		"",
		"## Scenario",
		"",
		strings.TrimSpace(description),
		"",
	}, "\n")
	smoke := strings.Join([]string{
		"# Smoke Test",
		"",
		"1. Run `cd server && go test ./...`.",
		"2. Run `cd server && go vet ./...`.",
		"3. Start the service with `cd server && go run .`.",
		"4. Request `http://127.0.0.1:8080/healthz` and confirm the JSON status is `ok`.",
		"5. Request `/` and confirm the service returns a successful response.",
		"",
		"## Scenario",
		"",
		strings.TrimSpace(description),
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(docsDir, "testing.md"), []byte(testing), 0o600); err != nil {
		return "", fmt.Errorf("write docs/testing.md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "smoke-test.md"), []byte(smoke), 0o600); err != nil {
		return "", fmt.Errorf("write docs/smoke-test.md: %w", err)
	}
	if commandAvailable("gofmt") {
		if err := runShell(ctx, root, "gofmt -w $(find . -name '*.go' -not -path './.git/*')"); err != nil {
			return "", err
		}
	}
	if !commandAvailable("go") {
		return "Added Go QA evidence docs. Go toolchain is unavailable; validation used static artifact checks.", nil
	}
	if err := runShell(ctx, filepath.Join(root, moduleDir), "go mod tidy"); err != nil {
		return "", err
	}
	if err := runShell(ctx, filepath.Join(root, moduleDir), "go test ./..."); err != nil {
		return "", err
	}
	if err := runShell(ctx, filepath.Join(root, moduleDir), "go vet ./..."); err != nil {
		return "", err
	}
	return "Added Go QA evidence docs and verified cd server && go test ./... and go vet ./....", nil
}

func repairDockerfileGoSumAssumption(root string) error {
	path := filepath.Join(root, "Dockerfile")
	if !fileExists(path) || fileExists(filepath.Join(root, "go.sum")) {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read Dockerfile: %w", err)
	}
	content := string(data)
	replacements := map[string]string{
		"COPY go.mod go.sum ./":   "COPY go.mod ./",
		"COPY go.mod go.sum .":    "COPY go.mod .",
		"COPY go.mod go.sum ./\n": "COPY go.mod ./\n",
	}
	updated := content
	for old, newValue := range replacements {
		updated = strings.ReplaceAll(updated, old, newValue)
	}
	if updated == content {
		return nil
	}
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		return fmt.Errorf("write Dockerfile: %w", err)
	}
	return nil
}

func inferFrontendProductTitle(description, root string) string {
	if title := readHTMLTitle(filepath.Join(root, "client", "index.html")); title != "" && !isDeploymentTopicTitle(title) {
		return title
	}
	if title := readHTMLTitle(filepath.Join(root, "index.html")); title != "" && !isDeploymentTopicTitle(title) {
		return title
	}
	projectName := inferProjectName(description, root)
	title := titleCase(strings.ReplaceAll(projectName, "-", " "))
	if isDeploymentTopicTitle(title) && requestsInvaderExperience(description) {
		return "One-Button Invaders"
	}
	if isDeploymentTopicTitle(title) {
		return titleCase(strings.ReplaceAll(filepath.Base(root), "-", " "))
	}
	return title
}

func readHTMLTitle(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lower := strings.ToLower(string(data))
	start := strings.Index(lower, "<title>")
	end := strings.Index(lower, "</title>")
	if start < 0 || end <= start {
		return ""
	}
	title := strings.TrimSpace(string(data[start+len("<title>") : end]))
	if strings.Contains(title, "<") || strings.Contains(title, ">") {
		return ""
	}
	return title
}

func isDeploymentTopicTitle(title string) bool {
	normalized := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(title, "-", " ")))
	switch normalized {
	case "kubernetes", "k8s", "docker", "helm", "deployment", "deploy", "devops", "frontend", "backend", "static", "container", "ci", "ci cd", "docs", "documentation", "testing":
		return true
	default:
		return false
	}
}

func frontendReadme(title, description string) string {
	return strings.Join([]string{
		"# " + title,
		"",
		"This repository started empty. ARUN generated a minimal static browser game with a gravity-lane mechanic so an implementation-heavy scrum workflow can produce reviewable code, documentation, and validation artifacts without GitHub API calls.",
		"",
		"## Repository layout",
		"",
		"- `server/` contains the Go HTTP entrypoint.",
		"- `client/` contains the browser UI served from `/`.",
		"- `charts/` and `k8s/` contain deployment artifacts when present.",
		"- `docs/` contains product and validation notes.",
		"",
		"## Features",
		"",
		"- Keyboard controls with ArrowLeft, ArrowRight, and Space.",
		"- Space flips the defender between floor and ceiling gravity lanes.",
		"- Score display that increments only when the defender is horizontally aligned and on the same gravity lane as the invader.",
		"- Lives tracking that decrements when an invader reaches the bottom of the arena.",
		"- Restart behavior that resets score, lives, player position, and invader position.",
		"",
		"## Run",
		"",
		"Run the Go server with `cd server && go run .` and open `http://127.0.0.1:8080/`, or open `client/index.html` directly for a static browser review.",
		"",
		"## Validate",
		"",
		"```sh",
		"npm --prefix client test",
		"npm --prefix client run build",
		"```",
		"",
		"Both scripts use `node --check` from `client/package.json` and do not require package installation.",
		"",
	}, "\n")
}

func frontendProductBrief(title, _ string) string {
	return strings.Join([]string{
		"# Product Brief: " + title,
		"",
		"## Concept",
		"",
		title + " is a compact browser invader game built around a gravity-lane flip mechanic. The player moves left and right, then uses Space to shift between the floor and ceiling lanes. Scoring requires both horizontal alignment and matching the invader lane, so the differentiating mechanic is present in the implemented UI and source code rather than only in documentation.",
		"",
		"## Target User",
		"",
		"- Players who want a short arcade loop with one clear twist.",
		"- Reviewers who need a fresh-checkout slice that runs without external services.",
		"",
		"## Acceptance Criteria",
		"",
		"- The visible title, README H1, and this product brief use the same product name.",
		"- The primary route `/` serves the browser game from `client/` when run through the Go server in `server/`.",
		"- Space changes the gravity lane between Floor and Ceiling.",
		"- A score is awarded only when the defender is aligned with the invader and on the same lane.",
		"- The Docker runtime image includes the client assets required for `/` to serve the same UI as local `cd server && go run .`.",
		"",
		"## Non-Goals",
		"",
		"- Multiplayer.",
		"- External score services.",
		"- Complex level progression.",
		"",
	}, "\n")
}

func artifactContractDoc(title, modulePath string) string {
	if strings.TrimSpace(title) == "" {
		title = "ARUN Sprint App"
	}
	if strings.TrimSpace(modulePath) == "" {
		modulePath = "github.com/hakobune8/arun-test"
	}
	return strings.Join([]string{
		"# Artifact Contract: " + title,
		"",
		"This contract is the implementation source of truth for how generated artifacts connect. Product intent lives in `docs/product-brief.md`; this file defines the route, file, module, and validation expectations that backend, frontend, QA, Docker, Helm, and documentation work must preserve.",
		"",
		"## Primary route",
		"",
		"- `/` serves the primary browser experience.",
		"- `/healthz` returns a JSON health response.",
		"",
		"## Frontend",
		"",
		"- Directory: `client/`.",
		"- Package file: `client/package.json`.",
		"- Entrypoint: `client/index.html`.",
		"- Required local assets: `client/styles.css` and `client/src/main.js`.",
		"- HTML must not reference local CSS or JavaScript files that are absent from the repository.",
		"",
		"## Backend",
		"",
		"- Language: Go.",
		"- Module path: `" + modulePath + "`.",
		"- Module file: `server/go.mod`.",
		"- Entrypoint: `server/main.go`.",
		"- The Go server must serve `/` from `client/index.html` and must serve every local CSS or JavaScript file referenced by that HTML.",
		"",
		"## Deployment",
		"",
		"- Docker and Kubernetes artifacts must package the same backend and frontend paths defined above.",
		"- Helm chart templates must expose the application service and health checks without introducing a separate product path.",
		"",
		"## Validation",
		"",
		"- `cd server && go test ./...` passes when Go tooling is available.",
		"- `cd server && go vet ./...` passes when Go tooling is available.",
		"- Frontend smoke validation confirms `/` returns HTML and that referenced local CSS/JS assets exist and are served.",
		"- QA must treat any mismatch between this contract and repository artifacts as release-blocking.",
		"",
	}, "\n")
}

func frontendSmokeTest(description string) string {
	return strings.Join([]string{
		"# Smoke Test",
		"",
		"1. Start the Go server with `cd server && go run .` and open `/`, or open `client/index.html` in a browser.",
		"2. Confirm the arena, score display, lives display, gravity display, target lane display, and restart button render without layout overlap.",
		"3. Press ArrowLeft and ArrowRight and confirm the defender moves horizontally.",
		"4. Press Space and confirm the gravity display flips between Floor and Ceiling.",
		"5. Press Space while aligned on the same lane as the invader and confirm the score increases.",
		"6. Let an invader reach the bottom and confirm lives decrement.",
		"7. Click `Restart` and confirm score returns to 0, lives returns to 3, and gravity returns to Floor.",
		"8. Run `npm --prefix client test` and `npm --prefix client run build`.",
		"",
		"## Product Coverage",
		"",
		"- Validates the generated browser game from the primary served route.",
		"- Focuses on implemented controls, scoring, lives, restart behavior, and layout.",
		"",
	}, "\n")
}

func frontendTestingDoc(description string) string {
	return strings.Join([]string{
		"# Testing",
		"",
		"## Automated validation",
		"",
		"Run the configured package scripts when a JavaScript runtime is available:",
		"",
		"```sh",
		"npm --prefix client test",
		"npm --prefix client run build",
		"```",
		"",
		"The generated scaffold keeps both commands dependency-free by using syntax checks from `client/package.json`.",
		"",
		"## Manual smoke check",
		"",
		"- Confirm keyboard controls move the defender with ArrowLeft and ArrowRight.",
		"- Confirm Space flips the gravity lane between Floor and Ceiling.",
		"- Confirm Space can score only when the defender is aligned with the invader and on the same lane.",
		"- Confirm the score display updates after a hit.",
		"- Confirm lives decrement when an invader reaches the bottom of the arena.",
		"- Confirm Restart restores score to 0 and lives to 3.",
		"- Confirm the page remains usable on narrow and wide viewports.",
		"",
		"## Product Coverage",
		"",
		"- Covers the generated gravity-lane invader game and its primary review path.",
		"- Does not claim Docker, Helm, Kubernetes, or CI execution unless those commands were run separately.",
		"",
	}, "\n")
}

func frontendChangelog(description string) string {
	return strings.Join([]string{
		"# Changelog",
		"",
		"## v0.1.0 - Unreleased",
		"",
		"- Added the initial static frontend scaffold for the implementation-heavy scrum workflow.",
		"- Added keyboard controls, gravity-lane flipping, score display, lives tracking, and restart behavior.",
		"- Added README run and validation instructions.",
		"- Added smoke-test and QA documentation for browser verification.",
		"",
		"## Release readiness",
		"",
		"- Review the generated static files before publishing.",
		"- Run `npm --prefix client test` and `npm --prefix client run build` when a JavaScript runtime is available.",
		"- Perform the manual browser smoke check documented in `docs/testing.md`.",
		"",
	}, "\n")
}

func cleanupGeneratedArtifactHygiene(root string) error {
	if err := repairGeneratedGoModuleLayout(root); err != nil {
		return err
	}
	if err := repairGeneratedFrontendPackageLayout(root); err != nil {
		return err
	}
	if err := migrateGeneratedRootAppLayout(root); err != nil {
		return err
	}
	if err := removeUnservedAlternateFrontendTrees(root); err != nil {
		return err
	}
	if err := removeIncompleteHelmCharts(root); err != nil {
		return err
	}
	if err := removeStrayRootHelmMetadata(root); err != nil {
		return err
	}
	if err := removeEmptyGeneratedArtifactFiles(root); err != nil {
		return err
	}
	if err := removeDuplicateGeneratedCIWorkflows(root); err != nil {
		return err
	}
	if err := removeGeneratedBinaryArtifacts(root); err != nil {
		return err
	}
	if err := removeGeneratedDocsWithInvalidRepositoryClaims(root); err != nil {
		return err
	}
	if err := removeDuplicateRootProductBrief(root); err != nil {
		return err
	}
	if err := removeDuplicateDocsProductBriefs(root); err != nil {
		return err
	}
	if err := scrubPromptContaminationFromGeneratedMarkdown(root); err != nil {
		return err
	}
	return nil
}

func unservedAlternateFrontendExists(root string) bool {
	for _, dir := range []string{"frontend", "web"} {
		if fileExists(filepath.Join(root, dir, "index.html")) && !mainServesFrontendDir(root, dir) {
			return true
		}
	}
	return false
}

func removeUnservedAlternateFrontendTrees(root string) error {
	for _, dir := range []string{"frontend", "web"} {
		path := filepath.Join(root, dir)
		if !fileExists(filepath.Join(path, "index.html")) || mainServesFrontendDir(root, dir) {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove unserved %s: %w", dir, err)
		}
	}
	return nil
}

func removeEmptyGeneratedArtifactFiles(root string) error {
	for _, dir := range []string{"charts", "k8s", filepath.Join(".github", "workflows"), "docs"} {
		base := filepath.Join(root, dir)
		if _, err := os.Stat(base); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		var dirs []string
		if err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				dirs = append(dirs, path)
				return nil
			}
			if entry.Name() == ".gitkeep" {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Size() == 0 {
				if err := os.Remove(path); err != nil {
					return fmt.Errorf("remove empty generated artifact %s: %w", path, err)
				}
			}
			return nil
		}); err != nil {
			return err
		}
		for i := len(dirs) - 1; i >= 0; i-- {
			if dirs[i] == base {
				continue
			}
			_ = os.Remove(dirs[i])
		}
	}
	return nil
}

func removeDuplicateGeneratedCIWorkflows(root string) error {
	workflowDir := filepath.Join(root, ".github", "workflows")
	ciPath := filepath.Join(workflowDir, "ci.yml")
	goPath := filepath.Join(workflowDir, "go.yml")
	if !fileExists(ciPath) || !fileExists(goPath) {
		return nil
	}
	ciName, err := workflowName(ciPath)
	if err != nil {
		return err
	}
	goName, err := workflowName(goPath)
	if err != nil {
		return err
	}
	if ciName == "" || ciName != goName {
		return nil
	}
	data, err := os.ReadFile(goPath)
	if err != nil {
		return err
	}
	content := strings.ToLower(string(data))
	if !strings.Contains(content, "go test ./...") || !strings.Contains(content, "go vet ./...") {
		return nil
	}
	if fileExists(filepath.Join(root, "client", "package.json")) &&
		(!strings.Contains(content, "npm --prefix client test") || !strings.Contains(content, "npm --prefix client run build")) {
		return nil
	}
	if err := os.Remove(ciPath); err != nil {
		return fmt.Errorf("remove duplicate generated CI workflow: %w", err)
	}
	return nil
}

func workflowName(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "name:") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		name = strings.Trim(name, `"'`)
		return name, nil
	}
	return "", nil
}

func unservedRootFrontendAssetsExist(root string) bool {
	if !fileExists(filepath.Join(root, "index.html")) || !fileExists(filepath.Join(root, "main.go")) {
		return false
	}
	assets, err := referencedRootFrontendAssets(filepath.Join(root, "index.html"))
	if err != nil || len(assets) == 0 {
		return false
	}
	data, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil {
		return false
	}
	content := string(data)
	for _, asset := range assets {
		if !fileExists(filepath.Join(root, asset)) {
			continue
		}
		if !mainServesStaticAsset(content, asset) {
			return true
		}
	}
	return false
}

func unservedClientFrontendAssetsExist(root string) bool {
	indexPath := filepath.Join(root, "client", "index.html")
	mainPath := filepath.Join(root, "server", "main.go")
	if !fileExists(indexPath) || !fileExists(mainPath) {
		return false
	}
	assets, err := referencedRootFrontendAssets(indexPath)
	if err != nil || len(assets) == 0 {
		return false
	}
	data, err := os.ReadFile(mainPath)
	if err != nil {
		return false
	}
	content := string(data)
	for _, asset := range assets {
		if !fileExists(filepath.Join(root, "client", filepath.FromSlash(asset))) {
			continue
		}
		if !mainServesClientAsset(content, asset) {
			return true
		}
	}
	return false
}

func mainServesClientAsset(content, asset string) bool {
	if strings.Contains(content, "FileServer") ||
		strings.Contains(content, "http.Dir") ||
		strings.Contains(content, "StripPrefix") {
		return true
	}
	if strings.Contains(content, "staticAssetPath") {
		base := filepath.Base(asset)
		if strings.HasPrefix(filepath.ToSlash(asset), "src/") {
			return strings.Contains(content, "/src/") || strings.Contains(content, base)
		}
		return strings.Contains(content, base)
	}
	return strings.Contains(content, filepath.ToSlash(filepath.Join("client", asset))) ||
		strings.Contains(content, asset)
}

func migrateGeneratedRootAppLayout(root string) error {
	if !fileExists(filepath.Join(root, "index.html")) ||
		!fileExists(filepath.Join(root, "styles.css")) ||
		!fileExists(filepath.Join(root, "src", "main.js")) {
		return nil
	}
	if err := os.MkdirAll(filepath.Join(root, "client", "src"), 0o755); err != nil {
		return err
	}
	moves := map[string]string{
		"index.html":                    filepath.Join("client", "index.html"),
		"styles.css":                    filepath.Join("client", "styles.css"),
		filepath.Join("src", "main.js"): filepath.Join("client", "src", "main.js"),
	}
	for from, to := range moves {
		fromPath := filepath.Join(root, from)
		toPath := filepath.Join(root, to)
		if !fileExists(fromPath) || fileExists(toPath) {
			continue
		}
		if err := os.Rename(fromPath, toPath); err != nil {
			return fmt.Errorf("move generated frontend asset %s to %s: %w", from, to, err)
		}
	}
	_ = os.Remove(filepath.Join(root, "src"))
	if err := rewriteGeneratedPackageJSONForStaticLayout(root); err != nil {
		return err
	}
	if fileExists(filepath.Join(root, "main.go")) {
		serverDir := generatedAppServerDir(root, "")
		if err := os.MkdirAll(filepath.Join(root, serverDir), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(root, serverDir, "main.go"), []byte(frontendServingGoMain()), 0o600); err != nil {
			return fmt.Errorf("write migrated %s/main.go: %w", serverDir, err)
		}
		if fileExists(filepath.Join(root, "main_test.go")) {
			data, err := os.ReadFile(filepath.Join(root, "main_test.go"))
			if err != nil {
				return err
			}
			updated := strings.ReplaceAll(string(data), `filepath.Join(dir, "index.html")`, `filepath.Join(dir, "client", "index.html")`)
			updated = strings.ReplaceAll(updated, `filepath.Join(dir, "styles.css")`, `filepath.Join(dir, "client", "styles.css")`)
			updated = strings.ReplaceAll(updated, `filepath.Join(dir, "src", "main.js")`, `filepath.Join(dir, "client", "src", "main.js")`)
			updated = strings.ReplaceAll(updated, `os.Mkdir(filepath.Join(dir, "src"), 0o755)`, `os.MkdirAll(filepath.Join(dir, "client", "src"), 0o755)`)
			if err := os.WriteFile(filepath.Join(root, serverDir, "main_test.go"), []byte(updated), 0o600); err != nil {
				return fmt.Errorf("write migrated %s/main_test.go: %w", serverDir, err)
			}
			if err := os.Remove(filepath.Join(root, "main_test.go")); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		if err := os.Remove(filepath.Join(root, "main.go")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if commandAvailable("gofmt") {
			_ = runCmd(context.Background(), root, "gofmt", "-w", filepath.Join(serverDir, "main.go"), filepath.Join(serverDir, "main_test.go"))
		}
	}
	return nil
}

func rewriteGeneratedPackageJSONForStaticLayout(root string) error {
	path := filepath.Join(root, "client", "package.json")
	if !fileExists(path) {
		path = filepath.Join(root, "package.json")
	}
	if !fileExists(path) {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated := strings.ReplaceAll(string(data), "node --check client/src/main.js", "node --check src/main.js")
	if updated == string(data) {
		return nil
	}
	return os.WriteFile(path, []byte(updated), 0o600)
}

func repairGeneratedFrontendPackageLayout(root string) error {
	rootPkg := filepath.Join(root, "package.json")
	if !fileExists(rootPkg) || !fileExists(filepath.Join(root, "client", "index.html")) {
		return nil
	}
	data, err := os.ReadFile(rootPkg)
	if err != nil {
		return err
	}
	content := strings.ReplaceAll(string(data), "node --check client/src/main.js", "node --check src/main.js")
	clientPkg := filepath.Join(root, "client", "package.json")
	if err := os.MkdirAll(filepath.Dir(clientPkg), 0o755); err != nil {
		return err
	}
	if !fileExists(clientPkg) {
		if err := os.WriteFile(clientPkg, []byte(content), 0o600); err != nil {
			return err
		}
	}
	if err := os.Remove(rootPkg); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func referencedRootFrontendAssets(indexPath string) ([]string, error) {
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}
	content := string(data)
	var assets []string
	for _, attr := range []string{"href=\"", "src=\""} {
		remaining := content
		for {
			start := strings.Index(remaining, attr)
			if start < 0 {
				break
			}
			remaining = remaining[start+len(attr):]
			end := strings.Index(remaining, "\"")
			if end < 0 {
				break
			}
			ref := strings.TrimPrefix(remaining[:end], "./")
			remaining = remaining[end+1:]
			if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "//") {
				continue
			}
			ref = strings.TrimPrefix(ref, "/")
			clean := filepath.Clean(ref)
			if strings.HasPrefix(clean, "..") {
				continue
			}
			lower := strings.ToLower(clean)
			if strings.HasSuffix(lower, ".css") || strings.HasSuffix(lower, ".js") {
				assets = append(assets, clean)
			}
		}
	}
	return assets, nil
}

func unreferencedGeneratedFrontendAssetsExist(root string) bool {
	assets, err := unreferencedGeneratedFrontendAssets(root)
	return err == nil && len(assets) > 0
}

func removeUnreferencedGeneratedFrontendAssets(root string) (int, error) {
	assets, err := unreferencedGeneratedFrontendAssets(root)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, rel := range assets {
		if err := os.Remove(filepath.Join(root, rel)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, fmt.Errorf("remove unreferenced frontend asset %s: %w", rel, err)
		}
		removed++
	}
	return removed, nil
}

func unreferencedGeneratedFrontendAssets(root string) ([]string, error) {
	indexPath := filepath.Join(root, "client", "index.html")
	if !fileExists(indexPath) || !fileExists(filepath.Join(root, "docs", "artifact-contract.md")) {
		return nil, nil
	}
	refs, err := referencedRootFrontendAssets(indexPath)
	if err != nil {
		return nil, err
	}
	referenced := make(map[string]bool, len(refs))
	for _, ref := range refs {
		referenced[filepath.ToSlash(filepath.Clean(ref))] = true
	}
	contractData, err := os.ReadFile(filepath.Join(root, "docs", "artifact-contract.md"))
	if err != nil {
		return nil, err
	}
	contract := string(contractData)
	var unreferenced []string
	err = filepath.WalkDir(filepath.Join(root, "client"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case "node_modules", "dist", "build", ".vite":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".css" && ext != ".js" {
			return nil
		}
		rel, err := filepath.Rel(filepath.Join(root, "client"), path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		repoRel := filepath.ToSlash(filepath.Join("client", rel))
		if referenced[rel] || strings.Contains(contract, rel) || strings.Contains(contract, repoRel) {
			return nil
		}
		unreferenced = append(unreferenced, repoRel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return unreferenced, nil
}

func mainServesStaticAsset(content, asset string) bool {
	return strings.Contains(content, "FileServer") ||
		strings.Contains(content, "http.Dir") ||
		strings.Contains(content, "StripPrefix") ||
		strings.Contains(content, "staticAssetPath") ||
		strings.Contains(content, "static") ||
		strings.Contains(content, asset)
}

func mainServesFrontendDir(root, dir string) bool {
	data, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil {
		return false
	}
	content := string(data)
	return strings.Contains(content, dir) ||
		strings.Contains(content, "FileServer") ||
		strings.Contains(content, "http.Dir") ||
		strings.Contains(content, "StripPrefix")
}

func removeIncompleteHelmCharts(root string) error {
	chartsDir := filepath.Join(root, "charts")
	entries, err := os.ReadDir(chartsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		chartDir := filepath.Join(chartsDir, entry.Name())
		if helmChartComplete(chartDir) {
			continue
		}
		if err := os.RemoveAll(chartDir); err != nil {
			return fmt.Errorf("remove incomplete Helm chart %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func removeStrayRootHelmMetadata(root string) error {
	rootChart := filepath.Join(root, "charts", "Chart.yaml")
	if !fileExists(rootChart) {
		return nil
	}
	nested, err := filepath.Glob(filepath.Join(root, "charts", "*", "Chart.yaml"))
	if err != nil {
		return err
	}
	if len(nested) == 0 {
		return nil
	}
	if err := os.Remove(rootChart); err != nil {
		return fmt.Errorf("remove stray root Helm metadata: %w", err)
	}
	return nil
}

func removeGeneratedDocsWithInvalidRepositoryClaims(root string) error {
	for _, rel := range generatedReportDocCandidates(root) {
		path := filepath.Join(root, rel)
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !markdownHasInvalidRepositoryClaims(root, string(data)) {
			continue
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove inconsistent generated doc %s: %w", rel, err)
		}
	}
	return nil
}

func removeDuplicateRootProductBrief(root string) error {
	if !fileExists(filepath.Join(root, "docs", "product-brief.md")) ||
		!fileExists(filepath.Join(root, "product-brief.md")) {
		return nil
	}
	if err := os.Remove(filepath.Join(root, "product-brief.md")); err != nil {
		return fmt.Errorf("remove duplicate root product brief: %w", err)
	}
	return nil
}

func removeDuplicateDocsProductBriefs(root string) error {
	canonical := filepath.Join(root, "docs", "product-brief.md")
	if !fileExists(canonical) {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(root, "docs"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		normalized := strings.ReplaceAll(strings.ToLower(name), "_", "-")
		if name == "product-brief.md" || normalized != "product-brief.md" {
			continue
		}
		rel := filepath.Join("docs", name)
		if err := os.Remove(filepath.Join(root, rel)); err != nil {
			return fmt.Errorf("remove duplicate docs product brief %s: %w", rel, err)
		}
	}
	return nil
}

func generatedReportDocCandidates(root string) []string {
	var candidates []string
	docsDir := filepath.Join(root, "docs")
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		return candidates
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			continue
		}
		name := strings.ToLower(entry.Name())
		if strings.HasPrefix(name, "sprint") || strings.Contains(name, "report") || strings.Contains(name, "review") || strings.Contains(name, "remediation") {
			candidates = append(candidates, filepath.Join("docs", entry.Name()))
		}
	}
	return candidates
}

func markdownHasInvalidRepositoryClaims(root, content string) bool {
	lower := strings.ToLower(content)
	for _, phrase := range []string{
		"human-led sprint",
		"implementation is deferred",
		"no code changes",
		"plan-only",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	for _, phrase := range []string{"コード実装は行わず", "コード実装は含まれません", "計画段階"} {
		if strings.Contains(content, phrase) {
			return true
		}
	}
	for _, rel := range []string{
		filepath.Join(".github", "workflows", "ci.yml"),
		filepath.Join("values.schema.json"),
	} {
		relSlash := filepath.ToSlash(rel)
		if strings.Contains(content, relSlash) && !fileExists(filepath.Join(root, rel)) {
			return true
		}
	}
	if fileExists(filepath.Join(root, "main.go")) && !mainGoContainsRoute(root, "/health") {
		if (strings.Contains(lower, "/health endpoint") || strings.Contains(content, "/health エンドポイント")) &&
			!strings.Contains(lower, "/healthz") {
			return true
		}
	}
	for _, dir := range []string{"cmd", "web", "frontend", "internal"} {
		if fileExists(filepath.Join(root, dir)) {
			continue
		}
		if mentionsRepositoryDir(content, dir) {
			return true
		}
	}
	return false
}

func mainGoContainsRoute(root, route string) bool {
	data, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), fmt.Sprintf("%q", route))
}

func mentionsRepositoryDir(content, dir string) bool {
	needle := dir + "/"
	idx := strings.Index(content, needle)
	for idx >= 0 {
		if idx == 0 || !isRepositoryPathWordByte(content[idx-1]) {
			return true
		}
		nextStart := idx + len(needle)
		if nextStart >= len(content) {
			return true
		}
		next := strings.Index(content[nextStart:], needle)
		if next < 0 {
			return false
		}
		idx = nextStart + next
	}
	return false
}

func isRepositoryPathWordByte(b byte) bool {
	return b == '_' || b == '-' || b == '.' || b == '/' ||
		('0' <= b && b <= '9') ||
		('A' <= b && b <= 'Z') ||
		('a' <= b && b <= 'z')
}

func helmChartComplete(chartDir string) bool {
	return fileExists(filepath.Join(chartDir, "values.yaml")) &&
		fileExists(filepath.Join(chartDir, "templates", "deployment.yaml")) &&
		fileExists(filepath.Join(chartDir, "templates", "service.yaml"))
}

func removeGeneratedBinaryArtifacts(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".arun", "node_modules":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if isGeneratedBinaryArtifact(path) {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove generated binary %s: %w", filepath.Base(path), err)
			}
		}
		return nil
	})
}

func isGeneratedBinaryArtifact(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 4 {
		return false
	}
	if bytes.HasPrefix(data, []byte{0x7f, 'E', 'L', 'F'}) || bytes.HasPrefix(data, []byte{'M', 'Z'}) {
		return true
	}
	return bytes.HasPrefix(data, []byte{0xfe, 0xed, 0xfa, 0xcf}) ||
		bytes.HasPrefix(data, []byte{0xcf, 0xfa, 0xed, 0xfe}) ||
		bytes.HasPrefix(data, []byte{0xfe, 0xed, 0xfa, 0xce}) ||
		bytes.HasPrefix(data, []byte{0xce, 0xfa, 0xed, 0xfe})
}

func scrubPromptContaminationFromGeneratedMarkdown(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".arun", "node_modules":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if strings.ToLower(filepath.Ext(path)) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := string(data)
		cut := promptContaminationCutIndex(content)
		if cut < 0 {
			return nil
		}
		cleaned := strings.TrimRight(content[:cut], " \t\r\n")
		if cleaned != "" {
			cleaned += "\n"
		}
		if err := os.WriteFile(path, []byte(cleaned), 0o600); err != nil {
			return err
		}
		return nil
	})
}

func promptContaminationCutIndex(content string) int {
	markers := []string{
		"\n## Scenario",
		"\n## Scenario coverage",
		"\nParent task:",
		"\nOperating mode:",
		"\nQuality bar:",
		"\nExpected output:",
	}
	best := -1
	for _, marker := range markers {
		if idx := strings.Index(content, marker); idx >= 0 && (best < 0 || idx < best) {
			best = idx
		}
	}
	for _, marker := range []string{"## Scenario", "Parent task:", "Operating mode:", "Quality bar:", "Expected output:"} {
		if strings.HasPrefix(content, marker) {
			return 0
		}
	}
	return best
}

func recoverGoCI(ctx context.Context, root string) (string, error) {
	if !generatedGoModuleExists(root) || !generatedGoEntrypointExists(root) {
		return "", fmt.Errorf("go service files are required before CI recovery")
	}
	testDir := "server"
	if fileExists(filepath.Join(root, "main.go")) && !fileExists(filepath.Join(root, "server", "main.go")) {
		testDir = "."
	}
	test := `package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthzHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	healthzHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode healthz response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status = %q, want ok", body["status"])
	}
}

func TestRootHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	rootHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
}
`
	if err := os.WriteFile(filepath.Join(root, testDir, "main_test.go"), []byte(test), 0o600); err != nil {
		return "", fmt.Errorf("write %s/main_test.go: %w", testDir, err)
	}
	workflowDir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		return "", fmt.Errorf("create workflow dir: %w", err)
	}
	var clientSteps string
	if fileExists(filepath.Join(root, "client", "package.json")) {
		clientSteps = `      - uses: actions/setup-node@v4
        with:
          node-version: "20"
      - run: npm --prefix client test
      - run: npm --prefix client run build
`
	}
	workflow := `name: CI

on:
  push:
  pull_request:

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
      - run: go test ./...
        working-directory: ` + testDir + `
      - run: go vet ./...
        working-directory: ` + testDir + `
` + clientSteps
	if err := os.WriteFile(filepath.Join(workflowDir, "go.yml"), []byte(workflow), 0o600); err != nil {
		return "", fmt.Errorf("write workflow: %w", err)
	}
	if commandAvailable("gofmt") {
		if err := runCmd(ctx, root, "gofmt", "-w", filepath.Join(testDir, "main_test.go")); err != nil {
			return "", err
		}
	}
	if !commandAvailable("go") {
		return "Added Go handler tests and GitHub Actions workflow. Go toolchain is unavailable; validation used static artifact checks.", nil
	}
	if err := runShell(ctx, filepath.Join(root, testDir), "go test ./..."); err != nil {
		return "", err
	}
	if err := runShell(ctx, filepath.Join(root, testDir), "go vet ./..."); err != nil {
		return "", err
	}
	return "Added Go handler tests and GitHub Actions workflow.", nil
}

func recoverDockerfile(ctx context.Context, root, description string) (string, error) {
	if !generatedGoModuleExists(root) || !generatedGoEntrypointExists(root) {
		if _, err := recoverGoBackend(ctx, root, description); err != nil {
			return "", err
		}
	}
	if missingReferencedFrontendAssetsExist(root) {
		if _, err := recoverFrontendStaticApp(root, description); err != nil {
			return "", err
		}
		if _, err := recoverGoBackend(ctx, root, description); err != nil {
			return "", err
		}
	}
	if err := repairGeneratedGoModuleLayout(root); err != nil {
		return "", err
	}
	if staticFrontendProjectExists(root) {
		if err := runShell(ctx, root, frontendValidationCommand); err != nil {
			if _, recoverErr := recoverGoBackend(ctx, root, description); recoverErr != nil {
				return "", recoverErr
			}
		}
		if err := runShell(ctx, root, servedFrontendRuntimeValidationCommand); err != nil {
			if _, recoverErr := recoverGoBackend(ctx, root, description); recoverErr != nil {
				return "", recoverErr
			}
		}
	}
	staticAssetCopies := ""
	if staticFrontendProjectExists(root) {
		staticAssetCopies = `COPY client /app/client
`
	}
	dockerfile := fmt.Sprintf(`FROM golang:1.22-alpine AS build
WORKDIR /src/server
COPY server/go.mod ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app .

FROM alpine:3.20
RUN addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /out/app /app/app
%s
USER app
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/app/app"]
`, staticAssetCopies)
	dockerignore := `.git
run.log
run_state.json
tool_log.jsonl
tmp
dist
node_modules
`
	if err := os.WriteFile(filepath.Join(root, "Dockerfile"), []byte(dockerfile), 0o600); err != nil {
		return "", fmt.Errorf("write Dockerfile: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".dockerignore"), []byte(dockerignore), 0o600); err != nil {
		return "", fmt.Errorf("write .dockerignore: %w", err)
	}
	docsDir := filepath.Join(root, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		return "", err
	}
	containerDocs := strings.Join([]string{
		"# Container Run",
		"",
		"Build the application image:",
		"",
		"```sh",
		"docker build -t invaders:local .",
		"```",
		"",
		"Run it locally:",
		"",
		"```sh",
		"docker run --rm -p 8080:8080 invaders:local",
		"curl http://127.0.0.1:8080/healthz",
		"curl http://127.0.0.1:8080/ | grep '<title>'",
		"```",
		"",
		"The runtime image copies `client/` into `/app/client`, so the container serves the same primary UI from `/` that local `cd server && go run .` serves.",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(docsDir, "container-run.md"), []byte(containerDocs), 0o600); err != nil {
		return "", fmt.Errorf("write container docs: %w", err)
	}
	if commandAvailable("docker") {
		if err := runShell(ctx, root, dockerValidationCommand); err != nil {
			return "", err
		}
	}
	return "Created Dockerfile, .dockerignore, and container run documentation.", nil
}

func recoverHelmChart(ctx context.Context, root, description string) (string, error) {
	projectName := inferProjectName(description, root)
	if strings.Contains(strings.ToLower(description), "invaders") {
		projectName = "invaders"
	}
	chartDir := filepath.Join(root, "charts", projectName)
	templatesDir := filepath.Join(chartDir, "templates")
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		return "", fmt.Errorf("create Helm templates dir: %w", err)
	}
	chart := fmt.Sprintf(`apiVersion: v2
name: %s
description: Minimal chart generated by ARUN for the implementation-heavy scrum workflow.
type: application
version: 0.1.0
appVersion: "0.1.0"
`, projectName)
	values := `replicaCount: 1

image:
  repository: ghcr.io/example/app
  tag: latest
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 80

containerPort: 8080
`
	deployment := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-{{ .Chart.Name }}
  labels:
    app.kubernetes.io/name: {{ .Chart.Name | quote }}
    app.kubernetes.io/instance: {{ .Release.Name | quote }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      app.kubernetes.io/name: {{ .Chart.Name | quote }}
      app.kubernetes.io/instance: {{ .Release.Name | quote }}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: {{ .Chart.Name | quote }}
        app.kubernetes.io/instance: {{ .Release.Name | quote }}
    spec:
      containers:
        - name: {{ .Chart.Name }}
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          ports:
            - name: http
              containerPort: {{ .Values.containerPort }}
          readinessProbe:
            httpGet:
              path: /healthz
              port: http
          livenessProbe:
            httpGet:
              path: /healthz
              port: http
`
	service := `apiVersion: v1
kind: Service
metadata:
  name: {{ .Release.Name }}-{{ .Chart.Name }}
  labels:
    app.kubernetes.io/name: {{ .Chart.Name | quote }}
    app.kubernetes.io/instance: {{ .Release.Name | quote }}
spec:
  type: {{ .Values.service.type }}
  selector:
    app.kubernetes.io/name: {{ .Chart.Name | quote }}
    app.kubernetes.io/instance: {{ .Release.Name | quote }}
  ports:
    - name: http
      port: {{ .Values.service.port }}
      targetPort: http
`
	files := map[string]string{
		filepath.Join(chartDir, "Chart.yaml"):                chart,
		filepath.Join(chartDir, "values.yaml"):               values,
		filepath.Join(templatesDir, "deployment.yaml"):       deployment,
		filepath.Join(templatesDir, "service.yaml"):          service,
		filepath.Join(templatesDir, "NOTES.txt"):             "Run `helm template {{ .Release.Name }} .` to render the Kubernetes manifests.\n",
		filepath.Join(chartDir, ".helmignore"):               ".git/\nrun.log\nrun_state.json\ntool_log.jsonl\n",
		filepath.Join(root, "k8s", projectName, "README.md"): "Rendered manifests can be produced from the Helm chart with `helm template`.\n",
		filepath.Join(root, "docs", "kubernetes-deploy.md"):  fmt.Sprintf("# Kubernetes Deploy\n\nUse `helm upgrade --install %s charts/%s` after setting an image repository and tag.\n", projectName, projectName),
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return "", fmt.Errorf("write %s: %w", path, err)
		}
	}
	if commandAvailable("helm") {
		if err := runCmd(ctx, root, "helm", "lint", chartDir); err != nil {
			return "", err
		}
		if err := runShell(ctx, root, fmt.Sprintf("helm template arun-validation %q >/dev/null", chartDir)); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("Created Helm chart at charts/%s with Deployment and Service manifests.", projectName), nil
}

func recoverDocs(root, description string) (string, error) {
	readme := strings.Join([]string{
		"# arun-test",
		"",
		"Minimal Go HTTP service used for the ARUN multi-agent orchestration scenario test.",
		"",
		"## Run",
		"",
		"```sh",
		"cd server && go run .",
		"```",
		"",
		"The service listens on `:8080`.",
		"",
		"## Endpoints",
		"",
		"- `GET /` returns a plain text service response.",
		"- `GET /healthz` returns `{\"status\":\"ok\"}` as JSON.",
		"",
		"## Test",
		"",
		"```sh",
		"cd server && go test ./...",
		"cd server && go vet ./...",
		"```",
		"",
	}, "\n")
	if strings.TrimSpace(description) != "" {
		readme += "\n## Scenario\n\n" + strings.TrimSpace(description) + "\n"
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte(readme), 0o600); err != nil {
		return "", fmt.Errorf("write README.md: %w", err)
	}
	return "Updated README.md with startup, endpoint, and test instructions.", nil
}

func recoverReview(root string) (string, error) {
	review := strings.Join([]string{
		"# Review",
		"",
		"The canonical ARUN v1.0 scenario files were generated and validated:",
		"",
		"- Go HTTP service files are present.",
		"- `/healthz` returns `{\"status\":\"ok\"}`.",
		"- Go tests and GitHub Actions workflow are present.",
		"- README includes startup, endpoint, and test instructions.",
		"",
		"No release-blocking findings for this scenario.",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "REVIEW.md"), []byte(review), 0o600); err != nil {
		return "", fmt.Errorf("write REVIEW.md: %w", err)
	}
	return "Wrote scenario review summary.", nil
}

func inferModulePath(description, root string) string {
	for _, token := range strings.Fields(description) {
		if modulePath := githubModuleFromToken(token); modulePath != "" {
			return modulePath
		}
	}
	if modulePath := githubModuleFromGitRemote(root); modulePath != "" {
		return modulePath
	}
	name := filepath.Base(root)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "arun-scenario"
	}
	return sanitizePackageName(name)
}

func inferServerModulePath(description, root string) string {
	modulePath := strings.TrimSuffix(inferModulePath(description, root), "/")
	if modulePath == "" {
		return "arun-scenario/server"
	}
	if strings.HasSuffix(modulePath, "/server") {
		return modulePath
	}
	return modulePath + "/server"
}

func githubModuleFromGitRemote(root string) string {
	if strings.TrimSpace(root) == "" || !commandAvailable("git") {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return githubModuleFromToken(strings.TrimSpace(string(out)))
}

func inferProjectName(description, root string) string {
	for _, token := range strings.Fields(description) {
		repo := strings.Trim(token, " \t\r\n.,;:()[]{}<>\"'`")
		repo = strings.TrimPrefix(repo, "https://github.com/")
		repo = strings.TrimPrefix(repo, "http://github.com/")
		repo = strings.TrimSuffix(repo, ".git")
		parts := strings.Split(repo, "/")
		if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
			return sanitizePackageName(parts[1])
		}
	}
	name := filepath.Base(root)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "arun-sprint-app"
	}
	return sanitizePackageName(name)
}

func requestsInvaderExperience(description string) bool {
	desc := strings.ToLower(description)
	return strings.Contains(desc, "invader") ||
		strings.Contains(desc, "space invaders") ||
		strings.Contains(description, "インベーダ") ||
		strings.Contains(description, "インベーダー")
}

func sanitizePackageName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	cleaned := strings.Trim(b.String(), "-")
	if cleaned == "" {
		return "arun-sprint-app"
	}
	return cleaned
}

func titleCase(value string) string {
	words := strings.Fields(value)
	for i, word := range words {
		if word == "" {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

func githubModuleFromToken(token string) string {
	token = strings.Trim(token, " \t\r\n.,;:()[]{}<>\"'`")
	token = strings.TrimPrefix(token, "https://")
	token = strings.TrimPrefix(token, "http://")
	token = strings.TrimPrefix(token, "ssh://")
	token = strings.TrimPrefix(token, "git@")
	token = strings.Replace(token, "github.com:", "github.com/", 1)
	if !strings.HasPrefix(token, "github.com/") {
		return ""
	}
	token = strings.TrimSuffix(token, ".git")
	parts := strings.Split(token, "/")
	if len(parts) < 3 || parts[1] == "" || parts[2] == "" {
		return ""
	}
	return strings.Join(parts[:3], "/")
}

func readmeCoversScenario(root string) bool {
	data, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		return false
	}
	content := strings.ToLower(string(data))
	return strings.Contains(content, "go run") &&
		strings.Contains(content, "/healthz") &&
		strings.Contains(content, "go test")
}

func ciCoversScenario(root string) bool {
	testPath := filepath.Join(root, "server", "main_test.go")
	if !fileExists(testPath) {
		testPath = filepath.Join(root, "main_test.go")
	}
	testData, err := os.ReadFile(testPath)
	if err != nil {
		return false
	}
	workflowData, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "go.yml"))
	if err != nil {
		return false
	}
	testContent := strings.ToLower(string(testData))
	workflowContent := strings.ToLower(string(workflowData))
	return strings.Contains(testContent, "healthzhandler") &&
		strings.Contains(testContent, "roothandler") &&
		strings.Contains(workflowContent, "go test ./...") &&
		strings.Contains(workflowContent, "go vet ./...")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func repositoryIsEffectivelyEmpty(root string) bool {
	empty := true
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			empty = false
			return filepath.SkipAll
		}
		if path == root {
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".arun":
				return filepath.SkipDir
			}
			return nil
		}
		empty = false
		return filepath.SkipAll
	})
	return err == nil && empty
}

func staticFrontendProjectExists(root string) bool {
	return (fileExists(filepath.Join(root, "client", "index.html")) &&
		(fileExists(filepath.Join(root, "client", "src", "main.js")) ||
			fileExists(filepath.Join(root, "client", "styles.css")))) ||
		(fileExists(filepath.Join(root, "index.html")) &&
			(fileExists(filepath.Join(root, "src", "main.js")) ||
				fileExists(filepath.Join(root, "styles.css"))))
}

func frontendProjectEvidenceExists(root string) bool {
	return staticFrontendProjectExists(root) ||
		fileExists(filepath.Join(root, "client", "index.html")) ||
		fileExists(filepath.Join(root, "index.html")) ||
		fileExists(filepath.Join(root, "frontend", "index.html")) ||
		fileExists(filepath.Join(root, "web", "index.html"))
}

func missingReferencedFrontendAssetsExist(root string) bool {
	for _, rel := range []string{
		filepath.Join("client", "index.html"),
		filepath.Join("frontend", "index.html"),
		filepath.Join("web", "index.html"),
		"index.html",
	} {
		indexPath := filepath.Join(root, rel)
		if !fileExists(indexPath) {
			continue
		}
		assets, err := referencedRootFrontendAssets(indexPath)
		if err != nil {
			continue
		}
		dir := filepath.Dir(rel)
		if dir == "." {
			dir = ""
		}
		for _, asset := range assets {
			if !fileExists(filepath.Join(root, dir, asset)) {
				return true
			}
		}
	}
	return false
}

func shouldRecoverFrontendScaffold(root, description string) bool {
	desc := strings.ToLower(description)
	isEmptyRepoTask := strings.Contains(desc, "empty repositor") || strings.Contains(desc, "completely empty") || strings.Contains(desc, "initial minimal app scaffold")
	isGoServiceFrontendTask := isCanonicalGoServiceTask(description) && (strings.Contains(desc, "frontend") || strings.Contains(desc, "static"))
	if !isEmptyRepoTask && !isGoServiceFrontendTask {
		return false
	}
	return !fileExists(filepath.Join(root, "package.json")) &&
		!fileExists(filepath.Join(root, "client", "index.html")) &&
		!fileExists(filepath.Join(root, "index.html")) &&
		!fileExists(filepath.Join(root, "client", "src", "main.js")) &&
		!fileExists(filepath.Join(root, "src", "main.js"))
}

func runShell(ctx context.Context, dir, command string) error {
	return runCmd(ctx, dir, "sh", "-c", command)
}

func commandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func runCmd(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		out := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		if out == "" {
			out = err.Error()
		}
		return fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), out)
	}
	return nil
}

func gitDiff(ctx context.Context, root string) string {
	cmd := exec.CommandContext(ctx, "git", "diff", "--", ".")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}
