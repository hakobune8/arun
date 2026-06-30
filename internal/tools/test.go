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

package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// TestTool runs test commands (e.g. go test) in the configured working
// directory.
type TestTool struct {
	WorkDir string
}

// NewTestTool creates a TestTool that runs commands in workDir.
func NewTestTool(workDir string) *TestTool {
	return &TestTool{WorkDir: workDir}
}

func (t *TestTool) Name() string { return "test" }

func (t *TestTool) Description() string { return "Run test commands (e.g. go test) and return output" }

func (t *TestTool) Run(ctx context.Context, input ToolInput) ToolOutput {
	command, _ := input["command"].(string)
	if command == "" {
		command = "go test ./..."
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	configureCommandCancel(cmd)
	cmd.Dir = t.WorkDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := ToolOutput{
		Success: err == nil,
		Data: map[string]string{
			"stdout":  strings.TrimSpace(stdout.String()),
			"stderr":  strings.TrimSpace(stderr.String()),
			"command": command,
		},
	}

	if err != nil {
		output.Error = fmt.Sprintf("test command failed: %v", err)
	}

	return output
}
