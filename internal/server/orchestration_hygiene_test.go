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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScrubRepositoryArtifacts_RemovesBinaryAndPromptBlocks(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "app"), []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	readme := "# Orbit Invaders\n\nProduct overview.\n\n## Scenario coverage\n\nParent task:\n新規性のあるインベーダーゲーム\n\nOperating mode: build-first\n\nQuality bar:\n- observable criteria\n\nExpected output:\n- docs\n"
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte(readme), 0o600); err != nil {
		t.Fatal(err)
	}
	doc := "# Testing\n\nValidation details.\n\n## Scenario\n\nParent task:\nmake it\n\nOperating mode: build-first\n\nQuality bar:\n- pass\n\nExpected output:\n- result\n"
	if err := os.WriteFile(filepath.Join(repo, "docs", "testing.md"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := scrubRepositoryArtifacts(repo)
	if err != nil {
		t.Fatalf("scrubRepositoryArtifacts() error = %v", err)
	}
	if len(result.Removed) != 1 || result.Removed[0] != "app" {
		t.Fatalf("removed = %+v, want app", result.Removed)
	}
	if len(result.Updated) != 2 {
		t.Fatalf("updated = %+v, want two markdown files", result.Updated)
	}
	if _, err := os.Stat(filepath.Join(repo, "app")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("binary stat err = %v, want not exist", err)
	}
	for _, file := range []string{"README.md", filepath.Join("docs", "testing.md")} {
		data, err := os.ReadFile(filepath.Join(repo, file))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if containsPromptContamination(text) || strings.Contains(text, "Parent task:") {
			t.Fatalf("%s still contains prompt contamination:\n%s", file, text)
		}
		if !strings.Contains(text, "#") {
			t.Fatalf("%s lost useful markdown content:\n%s", file, text)
		}
	}
}
