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

package server

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type repositoryHygieneResult struct {
	Removed []string
	Updated []string
}

func scrubRepositoryArtifacts(root string) (repositoryHygieneResult, error) {
	var result repositoryHygieneResult
	root = strings.TrimSpace(root)
	if root == "" {
		return result, fmt.Errorf("missing repository workspace")
	}
	if changed, err := ensureRepositoryGitignore(root); err != nil {
		return result, err
	} else if changed {
		result.Updated = append(result.Updated, ".gitignore")
	}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		name := entry.Name()
		if entry.IsDir() {
			switch name {
			case ".git", "node_modules", "vendor":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if isGeneratedArtifactNoise(path, entry, rel) {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("remove generated artifact %s: %w", rel, err)
			}
			result.Removed = append(result.Removed, rel)
			return nil
		}
		if strings.EqualFold(filepath.Ext(name), ".md") {
			updated, changed, err := scrubPromptBlockFromMarkdown(path)
			if err != nil {
				return err
			}
			if changed {
				result.Updated = append(result.Updated, rel)
			}
			if updated != "" && containsPromptContamination(updated) {
				return fmt.Errorf("prompt contamination remains in %s", rel)
			}
		}
		return nil
	}); err != nil {
		return result, err
	}
	return result, nil
}

func ensureRepositoryGitignore(root string) (bool, error) {
	if !generatedRepositoryNeedsGitignore(root) {
		return false, nil
	}
	path := filepath.Join(root, ".gitignore")
	var existing string
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	} else if !os.IsNotExist(err) {
		return false, err
	}
	lines := gitignoreLineSet(existing)
	patterns := []string{
		"server/server",
		"tmp/",
		"dist/",
		"node_modules/",
		"client/node_modules/",
		"client/dist/",
		"run.log",
		"run_state.json",
		"tool_log.jsonl",
	}
	var missing []string
	for _, pattern := range patterns {
		if !lines[pattern] {
			missing = append(missing, pattern)
		}
	}
	if len(missing) == 0 {
		return false, nil
	}
	var b strings.Builder
	trimmed := strings.TrimRight(existing, "\n")
	if trimmed != "" {
		b.WriteString(trimmed)
		b.WriteString("\n\n")
	}
	b.WriteString("# Local build and ARUN run artifacts\n")
	for _, pattern := range missing {
		b.WriteString(pattern)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return false, err
	}
	return true, nil
}

func generatedRepositoryNeedsGitignore(root string) bool {
	for _, rel := range []string{
		"go.mod",
		"package.json",
		"server",
		"client",
		filepath.Join("server", "go.mod"),
		filepath.Join("client", "package.json"),
		"Dockerfile",
		"charts",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			return true
		}
	}
	return false
}

func gitignoreLineSet(content string) map[string]bool {
	lines := make(map[string]bool)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines[line] = true
	}
	return lines
}

func isGeneratedArtifactNoise(path string, entry fs.DirEntry, rel string) bool {
	if isEmptyGeneratedArtifact(entry, rel) {
		return true
	}
	return isCompiledBinaryArtifact(path, entry)
}

func isEmptyGeneratedArtifact(entry fs.DirEntry, rel string) bool {
	info, err := entry.Info()
	if err != nil || info.IsDir() || info.Size() != 0 {
		return false
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if strings.HasSuffix(rel, "/.gitkeep") || rel == ".gitkeep" {
		return false
	}
	for _, prefix := range []string{
		"client/",
		"server/",
		"charts/",
		"k8s/",
		"docs/",
		".github/workflows/",
	} {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	return false
}

func isCompiledBinaryArtifact(path string, entry fs.DirEntry) bool {
	info, err := entry.Info()
	if err != nil || info.IsDir() || info.Size() == 0 {
		return false
	}
	if info.Size() > 256*1024*1024 {
		return true
	}
	data, err := readFilePrefix(path, 8)
	if err != nil {
		return false
	}
	if bytes.HasPrefix(data, []byte{0x7f, 'E', 'L', 'F'}) {
		return true
	}
	if bytes.HasPrefix(data, []byte("MZ")) {
		return true
	}
	if len(data) >= 4 {
		magic := data[:4]
		for _, want := range [][]byte{
			{0xfe, 0xed, 0xfa, 0xce},
			{0xfe, 0xed, 0xfa, 0xcf},
			{0xce, 0xfa, 0xed, 0xfe},
			{0xcf, 0xfa, 0xed, 0xfe},
			{0xca, 0xfe, 0xba, 0xbe},
		} {
			if bytes.Equal(magic, want) {
				return true
			}
		}
	}
	return false
}

func readFilePrefix(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close() //nolint:errcheck // read-only best effort close
	buf := make([]byte, limit)
	n, err := file.Read(buf)
	if err != nil && n == 0 {
		return nil, err
	}
	return buf[:n], nil
}

func scrubPromptBlockFromMarkdown(path string) (content string, changed bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, fmt.Errorf("read markdown %s: %w", filepath.Base(path), err)
	}
	content = string(data)
	if !containsPromptContamination(content) {
		return content, false, nil
	}
	cut := promptBlockCutIndex(content)
	if cut < 0 {
		return content, false, nil
	}
	cleaned := strings.TrimRight(content[:cut], " \t\r\n")
	if cleaned != "" {
		cleaned += "\n"
	}
	if err := os.WriteFile(path, []byte(cleaned), 0o600); err != nil {
		return "", false, fmt.Errorf("write cleaned markdown %s: %w", filepath.Base(path), err)
	}
	return cleaned, true, nil
}

func containsPromptContamination(content string) bool {
	markers := []string{
		"Parent task:",
		"Operating mode:",
		"Quality bar:",
		"Expected output:",
	}
	for _, marker := range markers {
		if !strings.Contains(content, marker) {
			return false
		}
	}
	return true
}

func promptBlockCutIndex(content string) int {
	candidates := []string{
		"\n## Scenario",
		"\n## Scenario coverage",
		"\nParent task:",
	}
	best := -1
	for _, marker := range candidates {
		if idx := strings.Index(content, marker); idx >= 0 && (best < 0 || idx < best) {
			best = idx
		}
	}
	if best >= 0 {
		return best
	}
	if strings.HasPrefix(content, "## Scenario") || strings.HasPrefix(content, "Parent task:") {
		return 0
	}
	return -1
}
