// Copyright 2026 AgentOS Authors
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
	if !isCanonicalGoServiceTask(subtask.Description) {
		return nil
	}
	switch subtask.AgentName {
	case "go-backend":
		return &QualityGate{
			RequiredFiles:      []string{"go.mod", "main.go"},
			ValidationCommands: []string{"go test ./...", "go vet ./..."},
			ContentChecks: []QualityContentCheck{{
				File:     "main.go",
				Contains: []string{"net/http", "/healthz", `"status"`},
			}},
		}
	case "ci-fixer":
		return &QualityGate{
			RequiredFiles:      []string{"main_test.go", filepath.Join(".github", "workflows", "go.yml")},
			ValidationCommands: []string{"go test ./..."},
			ContentChecks: []QualityContentCheck{{
				File:     filepath.Join(".github", "workflows", "go.yml"),
				Contains: []string{"go test ./..."},
			}},
		}
	case "docs":
		return &QualityGate{
			RequiredFiles: []string{"README.md"},
			ContentChecks: []QualityContentCheck{{
				File:     "README.md",
				Contains: []string{"/healthz", "go test", "go run"},
			}},
		}
	case "reviewer":
		return nil
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
