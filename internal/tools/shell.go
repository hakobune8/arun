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
	"time"

	"github.com/kazyamaz200/agentos/internal/safety"
)

// ShellTool executes shell commands subject to a command security policy.
type ShellTool struct {
	CommandPolicy *safety.CommandPolicy
	WorkDir       string
}

// NewShellTool creates a ShellTool that checks commands against policy and
// runs them in workDir.
func NewShellTool(policy *safety.CommandPolicy, workDir string) *ShellTool {
	return &ShellTool{
		CommandPolicy: policy,
		WorkDir:       workDir,
	}
}

func (t *ShellTool) Name() string { return "shell" }

func (t *ShellTool) Run(ctx context.Context, input ToolInput) ToolOutput {
	command, _ := input["command"].(string)
	if command == "" {
		return ToolOutput{Success: false, Error: "command is required"}
	}

	timeoutSec, _ := input["timeout"].(int)
	if timeoutSec <= 0 {
		timeoutSec = 60
	}

	ok, denied := t.CommandPolicy.Check(command)
	if !ok {
		return ToolOutput{
			Success: false,
			Error:   fmt.Sprintf("command denied by policy (matched: %q)", denied),
		}
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	//nolint:gosec // Shell intentionally used for pipe/redirect support; security enforced by CommandPolicy
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if t.WorkDir != "" {
		cmd.Dir = t.WorkDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := ToolOutput{
		Success: err == nil,
		Data: map[string]string{
			"stdout": strings.TrimSpace(stdout.String()),
			"stderr": strings.TrimSpace(stderr.String()),
		},
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			output.Error = fmt.Sprintf("command timed out after %d seconds", timeoutSec)
		} else {
			output.Error = fmt.Sprintf("command failed: %v", err)
		}
	}

	return output
}
