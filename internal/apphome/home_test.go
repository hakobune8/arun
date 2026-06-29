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

package apphome

import (
	"path/filepath"
	"testing"
)

func TestDir_UsesAgentOSHome(t *testing.T) {
	expected := t.TempDir()
	t.Setenv("AGENTOS_HOME", expected)

	if got := Dir(); got != expected {
		t.Fatalf("Dir() = %q, want %q", got, expected)
	}
}

func TestSubdirectories(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AGENTOS_HOME", root)

	if got := RunsDir(); got != filepath.Join(root, "runs") {
		t.Fatalf("RunsDir() = %q", got)
	}
	if got := VectorsDir(); got != filepath.Join(root, "vectors") {
		t.Fatalf("VectorsDir() = %q", got)
	}
}
