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

const frontendProjectPresenceCommand = `sh -c 'test -f client/package.json || test -f package.json || test -f client/index.html || test -f index.html || find client src app pages components assets public styles -type f \( -name "*.js" -o -name "*.jsx" -o -name "*.ts" -o -name "*.tsx" -o -name "*.vue" -o -name "*.svelte" -o -name "*.css" -o -name "*.html" \) -print -quit 2>/dev/null | grep -q .'`

const frontendPackageValidationCommand = `sh -c 'pkgdir=; [ -f client/package.json ] && pkgdir=client; [ -z "$pkgdir" ] && [ -f package.json ] && pkgdir=.; if [ -n "$pkgdir" ]; then PM=npm; [ -f "$pkgdir/pnpm-lock.yaml" ] && PM=pnpm; [ -f "$pkgdir/yarn.lock" ] && PM=yarn; [ -f "$pkgdir/bun.lockb" ] && PM=bun; if ! command -v node >/dev/null 2>&1 || ! command -v "$PM" >/dev/null 2>&1; then test -f docs/smoke-test.md || test -f docs/testing.md || test -f README.md || test -f client/src/main.js || test -f src/main.js || test -f client/index.html || test -f index.html; exit; fi; ran=0; for script in lint typecheck test build; do if (cd "$pkgdir" && node -e "const p=require(\"./package.json\"); process.exit(p.scripts&&p.scripts[process.argv[1]]?0:1)" "$script"); then ran=1; case "$PM:$script" in yarn:*) (cd "$pkgdir" && yarn "$script");; pnpm:*) (cd "$pkgdir" && pnpm "$script");; bun:*) (cd "$pkgdir" && bun run "$script");; *) (cd "$pkgdir" && npm run "$script");; esac; fi; done; test "$ran" = 1 || test -f docs/smoke-test.md || test -f README.md; fi'`

const frontendPackageLayoutValidationCommand = `sh -c 'if [ -f server/go.mod ] && [ -f client/index.html ]; then test -f client/package.json || { echo "client/package.json is required for separated generated frontend assets"; exit 1; }; test ! -f package.json || { echo "root package.json conflicts with separated client/package.json layout"; exit 1; }; fi'`

const frontendAssetHygieneValidationCommand = `sh -c 'if [ -f client/index.html ] && [ -f docs/artifact-contract.md ]; then tmp=$(mktemp); trap "rm -f $tmp" EXIT; grep -Eo "(href|src)=\"[^\"]+\\.(css|js)([#?][^\"]*)?\"" client/index.html | sed -E "s/^(href|src)=\"([^\"#?]+).*/\\2/" | sed -E "s#^\\./##; s#^/##; s#^client/##" >"$tmp"; find client -type f \( -name "*.css" -o -name "*.js" \) -not -path "*/node_modules/*" -print | while read -r asset; do rel=${asset#client/}; grep -Fxq "$rel" "$tmp" && continue; grep -Fq "$asset" docs/artifact-contract.md && continue; grep -Fq "$rel" docs/artifact-contract.md && continue; echo "$asset is not referenced by client/index.html or documented in docs/artifact-contract.md"; exit 1; done; fi'`

const generatedArtifactHygieneValidationCommand = `sh -c 'empty=$(find client server charts k8s .github/workflows docs -type f -empty ! -name ".gitkeep" -print -quit 2>/dev/null || true); test -z "$empty" || { echo "$empty is an empty generated artifact"; exit 1; }; if find charts -mindepth 2 -name Chart.yaml -print -quit 2>/dev/null | grep -q .; then stray=$(find charts -maxdepth 1 -type f \( -name Chart.yaml -o -name values.yaml -o -name values.schema.json -o -name Chart.lock -o -name .helmignore \) -print -quit 2>/dev/null || true); test -z "$stray" || { echo "$stray conflicts with chart subdirectories"; exit 1; }; fi; for chart in charts/*/Chart.yaml; do [ -f "$chart" ] || continue; dir=$(basename "$(dirname "$chart")"); cname=$(sed -n "s/^name:[[:space:]]*//p" "$chart" | head -n1 | tr -d "\"'"'"'" | tr "[:upper:]" "[:lower:]"); case "$dir:$cname" in name:*|app:*|chart:*|sample:*|example:*|*:name|*:app|*:chart|*:sample|*:example) echo "$chart uses placeholder chart name $dir/$cname"; exit 1;; esac; done; if [ -d .github/workflows ]; then names=$(for f in .github/workflows/*.yml .github/workflows/*.yaml; do [ -f "$f" ] || continue; sed -n "s/^name:[[:space:]]*//p" "$f" | head -n1; done); dup=$(printf "%s\n" "$names" | sed "/^$/d" | sort | uniq -d | head -n1); test -z "$dup" || { echo "duplicate GitHub Actions workflow name: $dup"; exit 1; }; fi'`

const frontendValidationCommand = `sh -c 'if [ -f main.go ] && { [ -f index.html ] || [ -d src ] || [ -d client ]; }; then echo "Go entrypoint and browser assets are mixed at repository root; use server/ and client/"; exit 1; fi; server_main=$(find server cmd -path "*/main.go" -print -quit 2>/dev/null); [ -z "$server_main" ] && [ -f main.go ] && server_main=main.go; for entry in frontend/index.html web/index.html client/index.html index.html; do if [ -f "$entry" ]; then dir=${entry%/*}; [ "$dir" = "$entry" ] && dir=.; if [ "$entry" != "index.html" ]; then test -n "$server_main" && grep -Eq "$dir|FileServer|http\\.Dir|StripPrefix|staticAssetPath" "$server_main" || { echo "$entry exists but is not served by the Go entrypoint"; exit 1; }; fi; for ref in $(grep -Eo "(href|src)=\"[^\"]+\\.(css|js)([#?][^\"]*)?\"" "$entry" | sed -E "s/^(href|src)=\"([^\"#?]+).*/\\2/" | sed -E "s#^\\./##"); do case "$ref" in http://*|https://*|//*) continue;; esac; clean=$(printf "%s" "$ref" | sed -E "s/[#?].*$//"); clean=${clean#/}; asset="$dir/$clean"; [ "$dir" = "." ] && asset="$clean"; test -f "$asset" || { echo "$entry references $clean but $asset is missing"; exit 1; }; if [ -n "$server_main" ]; then server_content=$(cat "$server_main"); if printf "%s" "$server_content" | grep -Eq "FileServer|http\\.Dir|StripPrefix"; then :; elif printf "%s" "$server_content" | grep -q "staticAssetPath"; then base=${clean##*/}; case "$clean" in src/*) printf "%s" "$server_content" | grep -Eq "/src/|$base" || { echo "$entry references $clean but Go staticAssetPath does not serve it"; exit 1; };; *) printf "%s" "$server_content" | grep -q "$base" || { echo "$entry references $clean but Go staticAssetPath does not serve it"; exit 1; };; esac; else printf "%s" "$server_content" | grep -Eq "$asset|$dir|client|static" || { echo "$entry references $clean but Go entrypoint does not serve static assets"; exit 1; }; fi; fi; done; fi; done'`

const servedFrontendRuntimeValidationCommand = `sh -c 'if [ "$(go env GOOS 2>/dev/null || echo unknown)" != "windows" ] && [ -f server/go.mod ] && [ -f server/main.go ] && [ -f client/index.html ] && command -v go >/dev/null 2>&1 && command -v curl >/dev/null 2>&1; then tmp=$(mktemp -d); port=$((20000 + ($$ % 20000))); pid=""; cleanup(){ if [ -n "$pid" ]; then kill "$pid" >/dev/null 2>&1 || true; wait "$pid" 2>/dev/null || true; fi; rm -rf "$tmp"; }; trap cleanup EXIT; (cd server && go build -o "$tmp/app" .); (cd server && PORT="$port" "$tmp/app" >"$tmp/server.log" 2>&1) & pid=$!; ready=0; i=0; while [ "$i" -lt 10 ]; do if curl -fsS "http://127.0.0.1:$port/" >/dev/null 2>&1; then ready=1; break; fi; i=$((i+1)); sleep 1; done; test "$ready" = 1 || { cat "$tmp/server.log" 2>/dev/null || true; exit 1; }; for ref in $(grep -Eo "(href|src)=\"[^\"]+\\.(css|js)([#?][^\"]*)?\"" client/index.html | sed -E "s/^(href|src)=\"([^\"#?]+).*/\\2/" | sed -E "s#^\\./##"); do case "$ref" in http://*|https://*|//*) continue;; esac; clean=$(printf "%s" "$ref" | sed -E "s/[#?].*$//"); clean=${clean#/}; curl -fsS "http://127.0.0.1:$port/$clean" >/dev/null || { echo "server runtime does not serve /$clean from client/index.html"; exit 1; }; done; fi'`

const productCoherenceValidationCommand = `sh -c 'brief=docs/product-brief.md; html=client/index.html; [ -f "$html" ] || html=index.html; if [ -f "$brief" ] && [ -f README.md ] && [ -f "$html" ]; then readme_title=$(sed -n "s/^# *//p" README.md | head -n1); brief_title=$(sed -n "s/^# \\(Product Brief: \\)\\{0,1\\}//p" "$brief" | head -n1 | sed -E "s/[[:space:]]+[—-].*$//"); html_title=$(sed -n "s/.*<title>\\([^<]*\\)<\\/title>.*/\\1/p" "$html" | head -n1 | sed -E "s/[[:space:]]+[—-].*$//"); norm(){ printf "%s" "$1" | tr "[:upper:]" "[:lower:]" | sed -E "s/product brief: //g; s/[^a-z0-9]+//g"; }; rt=$(norm "$readme_title"); bt=$(norm "$brief_title"); ht=$(norm "$html_title"); if [ -n "$rt" ] && [ -n "$bt" ] && [ "$rt" != "$bt" ]; then echo "README title $readme_title does not match product brief $brief_title"; exit 1; fi; if [ -n "$ht" ] && [ -n "$bt" ] && [ "$ht" != "$bt" ]; then echo "HTML title $html_title does not match product brief $brief_title"; exit 1; fi; fi'`

const artifactContractValidationCommand = `sh -c 'contract=docs/artifact-contract.md; if [ -f docs/product-brief.md ] || [ -f "$contract" ]; then test -f "$contract" || { echo "docs/artifact-contract.md is required for generated app artifacts"; exit 1; }; check(){ label=$1; shift; for term in "$@"; do grep -qi "$term" "$contract" && return 0; done; echo "artifact contract missing $label section"; exit 1; }; check "primary route" "Primary route" "提供ルート" "ルート"; check "frontend" "Frontend" "フロントエンド" "client/"; check "backend" "Backend" "バックエンド" "server/"; check "validation" "Validation" "バリデーション" "検証"; fi'`

const qaEvidenceCommand = `sh -c 'test -f docs/testing.md || test -f docs/smoke-test.md || test -f README.md || find test tests e2e cypress playwright -type f -print -quit 2>/dev/null | grep -q .'`

const qaValidationCommand = `sh -c 'moddir=; [ -f server/go.mod ] && moddir=server; [ -z "$moddir" ] && [ -f go.mod ] && moddir=.; if [ -n "$moddir" ]; then if command -v go >/dev/null 2>&1; then (cd "$moddir" && pkgs=$(go list ./...) && test -n "$pkgs" && go test ./...); else main=server/main.go; [ "$moddir" = "." ] && main=main.go; test -f "$main" && grep -q "/healthz" "$main"; fi; elif [ ! -f client/package.json ] && [ ! -f package.json ]; then test -f docs/testing.md || test -f docs/smoke-test.md || test -f README.md; fi'`

const docsValidationCommand = `sh -c 'if grep -R --include="*.md" -Eiq "human-led sprint|コード実装は行わず|コード実装は含まれません|計画段階|implementation is deferred|no code changes|plan-only" docs 2>/dev/null; then echo "documentation defers required implementation"; exit 1; fi; if [ ! -f .github/workflows/ci.yml ] && grep -R --include="*.md" -F ".github/workflows/ci.yml" README.md docs 2>/dev/null; then echo "documentation mentions missing .github/workflows/ci.yml"; exit 1; fi; if [ -f main.go ] && ! grep -q "\"/health\"" main.go && grep -R --include="*.md" -E "/health endpoint|/health エンドポイント" README.md docs 2>/dev/null | grep -vq "/healthz"; then echo "documentation mentions /health but main.go does not serve it"; exit 1; fi; for dir in cmd web frontend internal; do if [ ! -d "$dir" ] && grep -R --include="*.md" -E "(^|[^[:alnum:]_./-])$dir/" README.md docs 2>/dev/null; then echo "documentation mentions missing $dir/ layout"; exit 1; fi; done'`

const reportOnlyValidationCommand = `sh -c 'if command -v git >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then changed=$(git diff --name-only -- client server Dockerfile charts k8s .github 2>/dev/null | head -n1); test -z "$changed" || { echo "report-only subtask modified implementation or deployment file: $changed"; exit 1; }; fi'`

const goModuleLayoutValidationCommand = `sh -c 'if [ -f go.mod ] && [ -f server/go.mod ]; then root_go=$(find . -maxdepth 1 -name "*.go" -print -quit); test -n "$root_go" || { echo "root go.mod conflicts with canonical server/go.mod"; exit 1; }; fi'`

const goTestValidationCommand = `sh -c 'moddir=.; [ -f server/go.mod ] && moddir=server; if command -v go >/dev/null 2>&1; then if [ "$moddir" = "." ] && [ -f go.mod ]; then nested=$(find . -path "./.git" -prune -o -mindepth 2 -name go.mod -print | head -n1); test -z "$nested" || { echo "nested Go module $nested hides packages from root go test ./..."; exit 1; }; fi; (cd "$moddir" && pkgs=$(go list ./...) && test -n "$pkgs" || { echo "go list ./... matched no packages in $moddir"; exit 1; }); (cd "$moddir" && go test ./...); else main=server/main.go; [ "$moddir" = "." ] && main=main.go; test -f "$moddir/go.mod" && test -f "$main" && grep -q "/healthz" "$main"; fi'`

const goVetValidationCommand = `sh -c 'moddir=.; [ -f server/go.mod ] && moddir=server; if command -v go >/dev/null 2>&1; then if [ "$moddir" = "." ] && [ -f go.mod ]; then nested=$(find . -path "./.git" -prune -o -mindepth 2 -name go.mod -print | head -n1); test -z "$nested" || { echo "nested Go module $nested hides packages from root go vet ./..."; exit 1; }; fi; (cd "$moddir" && pkgs=$(go list ./...) && test -n "$pkgs" || { echo "go list ./... matched no packages in $moddir"; exit 1; }); (cd "$moddir" && go vet ./...); else main=server/main.go; [ "$moddir" = "." ] && main=main.go; test -f "$moddir/go.mod" && test -f "$main" && grep -q "net/http" "$main"; fi'`

const goBuildValidationCommand = `sh -c 'moddir=.; [ -f server/go.mod ] && moddir=server; if command -v go >/dev/null 2>&1; then (cd "$moddir" && pkgs=$(go list ./...) && test -n "$pkgs" || { echo "go list ./... matched no packages in $moddir"; exit 1; }); (cd "$moddir" && go build ./...); else main=server/main.go; [ "$moddir" = "." ] && main=main.go; test -f "$moddir/go.mod" && test -f "$main"; fi'`

const goModTidyValidationCommand = `sh -c 'moddir=.; [ -f server/go.mod ] && moddir=server; if command -v go >/dev/null 2>&1; then (cd "$moddir" && go mod tidy -diff); else test -f "$moddir/go.mod"; fi'`

const dockerValidationCommand = `sh -c 'test -f Dockerfile && grep -Eiq "^FROM[[:space:]]" Dockerfile && if [ -d client ]; then grep -Eq "COPY --from=.*client[[:space:]]+/app/client|COPY[[:space:]]+client[[:space:]]+/app/client" Dockerfile; elif [ -f index.html ] || [ -d src ]; then grep -Eq "COPY --from=.*index.html[[:space:]]+/app/index.html|COPY[[:space:]]+index.html[[:space:]]+/app/index.html" Dockerfile && grep -Eq "COPY --from=.*src[[:space:]]+/app/src|COPY[[:space:]]+src[[:space:]]+/app/src" Dockerfile; fi && if [ "$(go env GOOS 2>/dev/null || echo unknown)" != "windows" ] && command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then docker build -t arun-validation .; fi'`

const helmValidationCommand = `sh -c 'set -e; if [ -d charts ]; then for dir in charts/*; do [ -d "$dir" ] || continue; if [ -d "$dir/templates" ] || [ -f "$dir/Chart.yaml" ]; then test -f "$dir/Chart.yaml"; fi; done; fi; charts=$(find . -path "*/Chart.yaml" -not -path "./.git/*" -print); test -n "$charts"; for chart in $charts; do dir=$(dirname "$chart"); test -f "$dir/values.yaml"; test -d "$dir/templates"; grep -R "^kind: Deployment" "$dir/templates" >/dev/null; grep -R "^kind: Service" "$dir/templates" >/dev/null; if command -v helm >/dev/null 2>&1; then rendered=$(mktemp); helm lint "$dir"; helm template arun-validation "$dir" >"$rendered"; test -s "$rendered"; grep -q "^kind: Deployment" "$rendered"; grep -q "^kind: Service" "$rendered"; rm -f "$rendered"; fi; done'`

const kubernetesValidationCommand = `sh -c 'manifest=$(find . \( -name "*.yaml" -o -name "*.yml" \) -not -path "./.git/*" -exec grep -El "^(apiVersion|kind):" {} + | head -n1); test -n "$manifest"; if command -v kubectl >/dev/null 2>&1; then kubectl apply --dry-run=client -f "$manifest" >/dev/null; fi'`

const opsValidationCommand = `sh -c 'test -f Dockerfile || find . \( -path "*/Chart.yaml" -o -name "*.yaml" -o -name "*.yml" \) -not -path "./.git/*" -print -quit | grep -q .'`

const ciWorkflowValidationCommand = `sh -c 'workflow=$(find .github/workflows -maxdepth 1 -type f \( -name "*.yml" -o -name "*.yaml" \) -print -quit 2>/dev/null); test -n "$workflow" || { echo "GitHub Actions workflow is required for CI work"; exit 1; }; content=$(cat .github/workflows/*.yml .github/workflows/*.yaml 2>/dev/null); printf "%s" "$content" | grep -Eq "go test|go vet|go build" || { echo "CI workflow must run Go validation"; exit 1; }; if [ -f client/package.json ]; then printf "%s" "$content" | grep -Eq "npm --prefix client (test|run test)|working-directory:[[:space:]]*client" || { echo "CI workflow must run client validation"; exit 1; }; printf "%s" "$content" | grep -Eq "npm --prefix client run build|npm run build" || { echo "CI workflow must run client build validation"; exit 1; }; fi'`

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
	case "analyst":
		if strings.Contains(strings.ToLower(subtask.Description), "product planning") {
			return &QualityGate{
				RequiredFiles:      []string{filepath.Join("docs", "product-brief.md"), filepath.Join("docs", "artifact-contract.md")},
				ValidationCommands: []string{artifactContractValidationCommand},
			}
		}
		return nil
	case "go-backend":
		if !isCanonicalGoServiceTask(subtask.Description) {
			return nil
		}
		return &QualityGate{
			RequiredFiles:      []string{filepath.Join("server", "go.mod"), filepath.Join("server", "main.go")},
			ValidationCommands: []string{generatedArtifactHygieneValidationCommand, goModuleLayoutValidationCommand, goTestValidationCommand, goVetValidationCommand},
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
				frontendAssetHygieneValidationCommand,
				generatedArtifactHygieneValidationCommand,
				frontendPackageLayoutValidationCommand,
				frontendPackageValidationCommand,
				servedFrontendRuntimeValidationCommand,
				productCoherenceValidationCommand,
				artifactContractValidationCommand,
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
				ValidationCommands: []string{docsValidationCommand, productCoherenceValidationCommand, artifactContractValidationCommand},
			}
		}
		return &QualityGate{
			RequiredFiles:      []string{"README.md"},
			ValidationCommands: []string{docsValidationCommand, productCoherenceValidationCommand, artifactContractValidationCommand},
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
			ValidationCommands: []string{reportOnlyValidationCommand},
			ContentChecks: []QualityContentCheck{{
				File:     "CHANGELOG.md",
				Contains: []string{"Changelog", "v"},
			}},
		}
	case "dependency-updater":
		return &QualityGate{
			RequiredFiles:      []string{filepath.Join("server", "go.mod")},
			ValidationCommands: []string{goModTidyValidationCommand, goTestValidationCommand},
		}
	case "qa":
		return &QualityGate{
			ValidationCommands: []string{
				goModuleLayoutValidationCommand,
				qaEvidenceCommand,
				frontendAssetHygieneValidationCommand,
				generatedArtifactHygieneValidationCommand,
				frontendPackageLayoutValidationCommand,
				frontendPackageValidationCommand,
				qaValidationCommand,
				productCoherenceValidationCommand,
				artifactContractValidationCommand,
			},
		}
	case "docker":
		return &QualityGate{
			RequiredFiles:      []string{"Dockerfile"},
			ValidationCommands: []string{frontendValidationCommand, frontendPackageLayoutValidationCommand, frontendPackageValidationCommand, servedFrontendRuntimeValidationCommand, dockerValidationCommand},
			ContentChecks: []QualityContentCheck{{
				File:     "Dockerfile",
				Contains: []string{"FROM"},
			}},
		}
	case "helm":
		return &QualityGate{
			ValidationCommands: []string{generatedArtifactHygieneValidationCommand, helmValidationCommand},
		}
	case "kubernetes":
		return &QualityGate{
			ValidationCommands: []string{generatedArtifactHygieneValidationCommand, kubernetesValidationCommand},
		}
	case "devops":
		commands := []string{generatedArtifactHygieneValidationCommand, opsValidationCommand}
		if requiresCIWorkflow(subtask.Description) {
			commands = append(commands, ciWorkflowValidationCommand)
		}
		return &QualityGate{ValidationCommands: commands}
	default:
		return nil
	}
}

func requiresCIWorkflow(description string) bool {
	desc := strings.ToLower(description)
	for _, term := range []string{"github actions", "continuous", "ci", "workflow"} {
		if strings.Contains(desc, term) {
			return true
		}
	}
	return false
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
