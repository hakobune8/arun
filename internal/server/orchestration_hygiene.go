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
		if isCompiledBinaryArtifact(path, entry) {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("remove compiled artifact %s: %w", rel, err)
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
