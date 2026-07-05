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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// QualityGate declares required outputs for a subtask.
type QualityGate struct {
	RequiredFiles      []string              `json:"required_files,omitempty"`
	ValidationCommands []string              `json:"validation_commands,omitempty"`
	ContentChecks      []QualityContentCheck `json:"content_checks,omitempty"`
}

// QualityContentCheck requires a file to contain one or more strings.
type QualityContentCheck struct {
	File     string   `json:"file"`
	Contains []string `json:"contains"`
}

const frontendProjectPresenceCommand = `sh -c 'test -f package.json || test -f client/index.html || test -f index.html || find client src app pages components assets public styles -type f \( -name "*.js" -o -name "*.jsx" -o -name "*.ts" -o -name "*.tsx" -o -name "*.vue" -o -name "*.svelte" -o -name "*.css" -o -name "*.html" \) -print -quit 2>/dev/null | grep -q .'`

const frontendValidationCommand = `sh -c 'if [ -f main.go ] && { [ -f index.html ] || [ -d src ] || [ -d client ]; }; then echo "Go entrypoint and browser assets are mixed at repository root; use server/ and client/"; exit 1; fi; server_main=$(find server cmd -path "*/main.go" -print -quit 2>/dev/null); [ -z "$server_main" ] && [ -f main.go ] && server_main=main.go; for entry in frontend/index.html web/index.html client/index.html; do if [ -f "$entry" ]; then dir=${entry%/*}; test -n "$server_main" && grep -Eq "$dir|FileServer|http\\.Dir|StripPrefix|staticAssetPath" "$server_main" || { echo "$entry exists but is not served by the Go entrypoint"; exit 1; }; fi; done; if [ -f index.html ] && [ -n "$server_main" ]; then for asset in $(grep -Eo "(href|src)=\"[^\"]+\\.(css|js)\"" index.html | sed -E "s/^(href|src)=\"\\.\\/?([^\"#?]+).*/\\2/"); do test -f "$asset" || continue; grep -Eq "FileServer|http\\.Dir|StripPrefix|staticAssetPath|client|static|$asset" "$server_main" || { echo "index.html references $asset but Go entrypoint does not serve static assets"; exit 1; }; done; fi; if [ -f package.json ]; then PM=npm; [ -f pnpm-lock.yaml ] && PM=pnpm; [ -f yarn.lock ] && PM=yarn; [ -f bun.lockb ] && PM=bun; if ! command -v node >/dev/null 2>&1 || ! command -v "$PM" >/dev/null 2>&1; then test -f docs/smoke-test.md || test -f docs/testing.md || test -f README.md || test -f client/src/main.js || test -f src/main.js || test -f client/index.html || test -f index.html; exit; fi; for script in lint typecheck test build; do if node -e "const p=require(\"./package.json\"); process.exit(p.scripts&&p.scripts[process.argv[1]]?0:1)" "$script"; then case "$PM:$script" in yarn:*) yarn "$script";; pnpm:*) pnpm "$script";; bun:*) bun run "$script";; *) npm run "$script";; esac; fi; done; fi'`

const qaEvidenceCommand = `sh -c 'test -f docs/testing.md || test -f docs/smoke-test.md || test -f README.md || find test tests e2e cypress playwright -type f -print -quit 2>/dev/null | grep -q .'`

const qaValidationCommand = `sh -c 'if [ -f package.json ]; then PM=npm; [ -f pnpm-lock.yaml ] && PM=pnpm; [ -f yarn.lock ] && PM=yarn; [ -f bun.lockb ] && PM=bun; if ! command -v node >/dev/null 2>&1 || ! command -v "$PM" >/dev/null 2>&1; then test -f docs/smoke-test.md || test -f docs/testing.md || test -f README.md; exit; fi; ran=0; for script in lint typecheck test build; do if node -e "const p=require(\"./package.json\"); process.exit(p.scripts&&p.scripts[process.argv[1]]?0:1)" "$script"; then ran=1; case "$PM:$script" in yarn:*) yarn "$script";; pnpm:*) pnpm "$script";; bun:*) bun run "$script";; *) npm run "$script";; esac; fi; done; test "$ran" = 1 || test -f docs/smoke-test.md || test -f README.md; elif [ -f go.mod ]; then if command -v go >/dev/null 2>&1; then go test ./...; else main=server/main.go; [ -f "$main" ] || main=main.go; test -f "$main" && grep -q "/healthz" "$main"; fi; else test -f docs/testing.md || test -f docs/smoke-test.md || test -f README.md; fi'`

const docsValidationCommand = `sh -c 'if [ -f docs/remediation_notes.md ] && grep -Eiq "human-led sprint|コード実装は行わず|計画段階|implementation is deferred|no code changes" docs/remediation_notes.md; then echo "documentation defers required implementation"; exit 1; fi; if [ -f main.go ] && ! grep -q "\"/health\"" main.go && grep -R --include="*.md" -E "/health endpoint|/health エンドポイント" README.md docs 2>/dev/null | grep -vq "/healthz"; then echo "documentation mentions /health but main.go does not serve it"; exit 1; fi; for dir in cmd web frontend internal; do if [ ! -d "$dir" ] && grep -R --include="*.md" -E "(^|[^[:alnum:]_./-])$dir/" README.md docs 2>/dev/null; then echo "documentation mentions missing $dir/ layout"; exit 1; fi; done'`

const goTestValidationCommand = `sh -c 'if command -v go >/dev/null 2>&1; then go test ./...; else main=server/main.go; [ -f "$main" ] || main=main.go; test -f go.mod && test -f "$main" && grep -q "/healthz" "$main"; fi'`

const goVetValidationCommand = `sh -c 'if command -v go >/dev/null 2>&1; then go vet ./...; else main=server/main.go; [ -f "$main" ] || main=main.go; test -f go.mod && test -f "$main" && grep -q "net/http" "$main"; fi'`

const goModTidyValidationCommand = `sh -c 'if command -v go >/dev/null 2>&1; then go mod tidy -diff; else test -f go.mod; fi'`

const dockerValidationCommand = `sh -c 'test -f Dockerfile && grep -Eiq "^FROM[[:space:]]" Dockerfile && if [ -d client ]; then grep -Eq "COPY --from=.*client[[:space:]]+/app/client|COPY[[:space:]]+client[[:space:]]+/app/client" Dockerfile; elif [ -f index.html ] || [ -d src ]; then grep -Eq "COPY --from=.*index.html[[:space:]]+/app/index.html|COPY[[:space:]]+index.html[[:space:]]+/app/index.html" Dockerfile && grep -Eq "COPY --from=.*src[[:space:]]+/app/src|COPY[[:space:]]+src[[:space:]]+/app/src" Dockerfile; fi && if [ "$(go env GOOS 2>/dev/null || echo unknown)" != "windows" ] && command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then docker build -t arun-validation .; fi'`

const helmValidationCommand = `sh -c 'set -e; if [ -d charts ]; then for dir in charts/*; do [ -d "$dir" ] || continue; if [ -d "$dir/templates" ] || [ -f "$dir/Chart.yaml" ]; then test -f "$dir/Chart.yaml"; fi; done; fi; charts=$(find . -path "*/Chart.yaml" -not -path "./.git/*" -print); test -n "$charts"; for chart in $charts; do dir=$(dirname "$chart"); test -f "$dir/values.yaml"; test -d "$dir/templates"; grep -R "^kind: Deployment" "$dir/templates" >/dev/null; grep -R "^kind: Service" "$dir/templates" >/dev/null; if command -v helm >/dev/null 2>&1; then rendered=$(mktemp); helm lint "$dir"; helm template arun-validation "$dir" >"$rendered"; test -s "$rendered"; grep -q "^kind: Deployment" "$rendered"; grep -q "^kind: Service" "$rendered"; rm -f "$rendered"; fi; done'`

const kubernetesValidationCommand = `sh -c 'manifest=$(find . \( -name "*.yaml" -o -name "*.yml" \) -not -path "./.git/*" -exec grep -El "^(apiVersion|kind):" {} + | head -n1); test -n "$manifest"; if command -v kubectl >/dev/null 2>&1; then kubectl apply --dry-run=client -f "$manifest" >/dev/null; fi'`

const opsValidationCommand = `sh -c 'test -f Dockerfile || find . \( -path "*/Chart.yaml" -o -name "*.yaml" -o -name "*.yml" \) -not -path "./.git/*" -print -quit | grep -q .'`

// QualityGateStatus reports the result of validating a subtask's gate.
type QualityGateStatus struct {
	Passed bool                     `json:"passed"`
	Checks []QualityGateCheckResult `json:"checks,omitempty"`
}

// QualityGateCheckResult reports one gate check.
type QualityGateCheckResult struct {
	Type    string `json:"type"`
	Target  string `json:"target"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

func qualityGateForSubtask(subtask *Subtask) *QualityGate {
	switch subtask.AgentName {
	case "go-backend":
		if !isCanonicalGoServiceTask(subtask.Description) {
			return nil
		}
		return &QualityGate{
			RequiredFiles:      []string{"go.mod", filepath.Join("server", "main.go")},
			ValidationCommands: []string{goTestValidationCommand, goVetValidationCommand},
			ContentChecks: []QualityContentCheck{{
				File:     filepath.Join("server", "main.go"),
				Contains: []string{"net/http", "/healthz", `"status"`},
			}},
		}
	case "frontend":
		return &QualityGate{
			ValidationCommands: []string{
				frontendProjectPresenceCommand,
				frontendValidationCommand,
			},
		}
	case "ci-fixer":
		if !isCanonicalGoServiceTask(subtask.Description) {
			return nil
		}
		return &QualityGate{
			RequiredFiles:      []string{filepath.Join("server", "main_test.go"), filepath.Join(".github", "workflows", "go.yml")},
			ValidationCommands: []string{goTestValidationCommand},
			ContentChecks: []QualityContentCheck{{
				File:     filepath.Join(".github", "workflows", "go.yml"),
				Contains: []string{"go test ./..."},
			}},
		}
	case "docs":
		if !isCanonicalGoServiceTask(subtask.Description) {
			return &QualityGate{
				ValidationCommands: []string{docsValidationCommand},
			}
		}
		return &QualityGate{
			RequiredFiles:      []string{"README.md"},
			ValidationCommands: []string{docsValidationCommand},
			ContentChecks: []QualityContentCheck{{
				File:     "README.md",
				Contains: []string{"/healthz", "go test", "go run"},
			}},
		}
	case "reviewer":
		return nil
	case "security":
		return &QualityGate{
			RequiredFiles:      []string{"SECURITY.md"},
			ValidationCommands: []string{goTestValidationCommand, goVetValidationCommand},
			ContentChecks: []QualityContentCheck{{
				File:     "SECURITY.md",
				Contains: []string{"Security"},
			}},
		}
	case "release-manager":
		return &QualityGate{
			RequiredFiles: []string{"CHANGELOG.md"},
			ContentChecks: []QualityContentCheck{{
				File:     "CHANGELOG.md",
				Contains: []string{"Changelog", "v"},
			}},
		}
	case "dependency-updater":
		return &QualityGate{
			RequiredFiles:      []string{"go.mod"},
			ValidationCommands: []string{goModTidyValidationCommand, goTestValidationCommand},
		}
	case "qa":
		return &QualityGate{
			ValidationCommands: []string{
				qaEvidenceCommand,
				qaValidationCommand,
			},
		}
	case "docker":
		return &QualityGate{
			RequiredFiles:      []string{"Dockerfile"},
			ValidationCommands: []string{dockerValidationCommand},
			ContentChecks: []QualityContentCheck{{
				File:     "Dockerfile",
				Contains: []string{"FROM"},
			}},
		}
	case "helm":
		return &QualityGate{
			ValidationCommands: []string{helmValidationCommand},
		}
	case "kubernetes":
		return &QualityGate{
			ValidationCommands: []string{kubernetesValidationCommand},
		}
	case "devops":
		return &QualityGate{
			ValidationCommands: []string{opsValidationCommand},
		}
	default:
		return nil
	}
}

func applyDefaultQualityGate(subtask *Subtask) {
	if subtask == nil || !subtask.QualityGate.empty() {
		return
	}
	subtask.QualityGate = qualityGateForSubtask(subtask)
}

func (g *QualityGate) empty() bool {
	return g == nil || (len(g.RequiredFiles) == 0 && len(g.ValidationCommands) == 0 && len(g.ContentChecks) == 0)
}

func validateQualityGate(ctx context.Context, root string, gate *QualityGate) QualityGateStatus {
	if gate.empty() {
		return QualityGateStatus{Passed: true}
	}
	status := QualityGateStatus{Passed: true}
	for _, file := range gate.RequiredFiles {
		result := QualityGateCheckResult{Type: "required_file", Target: file, Passed: true}
		path, err := safeRepoPath(root, file)
		if err != nil {
			result.Passed = false
			result.Message = err.Error()
		} else if info, err := os.Stat(path); err != nil {
			result.Passed = false
			result.Message = "file is missing"
		} else if info.IsDir() {
			result.Passed = false
			result.Message = "path is a directory"
		}
		status.add(result)
	}
	for _, check := range gate.ContentChecks {
		result := QualityGateCheckResult{Type: "content", Target: check.File, Passed: true}
		path, err := safeRepoPath(root, check.File)
		if err != nil {
			result.Passed = false
			result.Message = err.Error()
			status.add(result)
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			result.Passed = false
			result.Message = "read file: " + err.Error()
			status.add(result)
			continue
		}
		content := string(data)
		var missing []string
		for _, want := range check.Contains {
			if !strings.Contains(content, want) {
				missing = append(missing, want)
			}
		}
		if len(missing) > 0 {
			result.Passed = false
			result.Message = "missing content: " + strings.Join(missing, ", ")
		}
		status.add(result)
	}
	for _, command := range gate.ValidationCommands {
		result := QualityGateCheckResult{Type: "command", Target: command, Passed: true}
		if err := runShell(ctx, root, command); err != nil {
			result.Passed = false
			result.Message = err.Error()
		}
		status.add(result)
	}
	return status
}

func (s *QualityGateStatus) add(result QualityGateCheckResult) {
	s.Checks = append(s.Checks, result)
	if !result.Passed {
		s.Passed = false
	}
}

func safeRepoPath(root, name string) (string, error) {
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("path must be relative")
	}
	clean := filepath.Clean(name)
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("path escapes repository")
	}
	path := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes repository")
	}
	return path, nil
}

func qualityGateError(status QualityGateStatus) string {
	var failed []string
	for _, check := range status.Checks {
		if check.Passed {
			continue
		}
		msg := check.Type + " " + check.Target
		if check.Message != "" {
			msg += ": " + check.Message
		}
		failed = append(failed, msg)
	}
	if len(failed) == 0 {
		return "quality gate failed"
	}
	return "quality gate failed: " + strings.Join(failed, "; ")
}
