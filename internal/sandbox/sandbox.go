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

package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
)

// Sandbox defines the interface for execution isolation.
// Implementations can use local directories, Docker containers, or
// remote workers. Runtime does not know which is in use.
type Sandbox interface {
	PrepareRun(taskID string) error
	RunPath() string
	SaveFile(name string, data []byte) error
	AbsPath(relative string) string
	RepoAbsPath(repoRelative string) string
	RootDir() string
	Type() string
}

// Config defines which sandbox backend to use and its options.
type Config struct {
	// Backend selects the implementation: "local", "docker".
	// Defaults to "local".
	Backend string `yaml:"backend"`

	// RootDir is the workspace root directory (local) or mount source (docker).
	RootDir string `yaml:"root_dir"`

	// Image is the Docker image to use (docker backend only).
	Image string `yaml:"image"`
}

// DefaultConfig returns a default sandbox configuration using local backend.
func DefaultConfig() Config {
	return Config{
		Backend: "local",
	}
}

// New creates a Sandbox from the given config.
func New(cfg Config) (Sandbox, error) {
	switch cfg.Backend {
	case "local":
		return NewLocalSandbox(cfg.RootDir), nil

	case "docker":
		return nil, fmt.Errorf("docker sandbox not yet implemented")

	default:
		return nil, fmt.Errorf("unknown sandbox backend: %q (options: local, docker)", cfg.Backend)
	}
}

// LocalSandbox implements Sandbox using the local filesystem.
type LocalSandbox struct {
	rootDir string
	RunsDir string
	TaskID  string
	RunDir  string
}

// NewLocalSandbox creates a LocalSandbox with the given project root directory.
func NewLocalSandbox(rootDir string) *LocalSandbox {
	return &LocalSandbox{rootDir: rootDir}
}

// Type returns "local" as the backend identifier.
func (s *LocalSandbox) Type() string { return "local" }

// RootDir returns the sandbox's root directory path.
func (s *LocalSandbox) RootDir() string { return s.rootDir }

// PrepareRun creates the run directory structure under ~/.agentos/runs for
// the given taskID.
func (s *LocalSandbox) PrepareRun(taskID string) error {
	s.TaskID = taskID
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	s.RunsDir = filepath.Join(homeDir, ".agentos", "runs")
	s.RunDir = filepath.Join(s.RunsDir, taskID)

	if err := os.MkdirAll(s.RunDir, 0o755); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}
	return nil
}

// RunPath returns the full path to the current run directory.
func (s *LocalSandbox) RunPath() string {
	return s.RunDir
}

// SaveFile writes data to a file named name inside the run directory.
func (s *LocalSandbox) SaveFile(name string, data []byte) error {
	path := filepath.Join(s.RunDir, name)
	return os.WriteFile(path, data, 0o600)
}

// AbsPath resolves a relative path against the workspace root.
func (s *LocalSandbox) AbsPath(relative string) string {
	return filepath.Join(s.rootDir, relative)
}

// RepoAbsPath resolves a repository-relative path against the workspace root.
// If the input is already absolute, it is returned unchanged.
func (s *LocalSandbox) RepoAbsPath(repoRelative string) string {
	if filepath.IsAbs(repoRelative) {
		return repoRelative
	}
	return filepath.Join(s.rootDir, repoRelative)
}
