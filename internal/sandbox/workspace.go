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

// Package sandbox provides sandboxed execution environments and workspace
// management for running agent tasks safely.
package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
)

// Workspace manages the filesystem layout for an agent task run, including
// root directory, run directories, and file path resolution.
type Workspace struct {
	RootDir   string
	RunsDir   string
	TaskID    string
	RunDir    string
}

// NewWorkspace creates a Workspace with the given project root directory.
func NewWorkspace(rootDir string) *Workspace {
	return &Workspace{RootDir: rootDir}
}

// PrepareRun creates the run directory structure under ~/.agentos/runs for
// the given taskID.
func (w *Workspace) PrepareRun(taskID string) error {
	w.TaskID = taskID
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	w.RunsDir = filepath.Join(homeDir, ".agentos", "runs")
	w.RunDir = filepath.Join(w.RunsDir, taskID)

	if err := os.MkdirAll(w.RunDir, 0o755); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}
	return nil
}

// RunPath returns the full path to the current run directory.
func (w *Workspace) RunPath() string {
	return w.RunDir
}

// SaveFile writes data to a file named name inside the run directory.
func (w *Workspace) SaveFile(name string, data []byte) error {
	path := filepath.Join(w.RunDir, name)
	return os.WriteFile(path, data, 0o600)
}

// AbsPath resolves a relative path against the workspace root.
func (w *Workspace) AbsPath(relative string) string {
	return filepath.Join(w.RootDir, relative)
}

// RepoAbsPath resolves a repository-relative path against the workspace root.
// If the input is already absolute, it is returned unchanged.
func (w *Workspace) RepoAbsPath(repoRelative string) string {
	if filepath.IsAbs(repoRelative) {
		return repoRelative
	}
	return filepath.Join(w.RootDir, repoRelative)
}
