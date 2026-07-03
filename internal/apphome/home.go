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

// Package apphome resolves ARUN state directories.
package apphome

import (
	"os"
	"path/filepath"
)

// Dir returns the root directory used for ARUN state. ARUN_HOME takes
// precedence so container deployments can mount state independently from the
// process user's home directory.
func Dir() string {
	if dir := os.Getenv("ARUN_HOME"); dir != "" {
		return dir
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".arun"
	}
	return filepath.Join(homeDir, ".arun")
}

// RunsDir returns the directory used for run artifacts.
func RunsDir() string {
	return filepath.Join(Dir(), "runs")
}

// VectorsDir returns the directory used for local vector indexes.
func VectorsDir() string {
	return filepath.Join(Dir(), "vectors")
}
