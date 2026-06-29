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

// GitTool provides a set of common git operations (diff, status, commit,
// branch, checkout, add, log) via a single Tool interface.
type GitTool struct {
	RepoPath string
}

// NewGitTool creates a GitTool that operates on the repository at repoPath.
func NewGitTool(repoPath string) *GitTool {
	return &GitTool{RepoPath: repoPath}
}

func (t *GitTool) Name() string { return "git" }

func (t *GitTool) Description() string { return "Run git operations (diff, status, commit, branch, checkout, add, log)" }

func (t *GitTool) Run(ctx context.Context, input ToolInput) ToolOutput {
	subcommand, _ := input["subcommand"].(string)
	args, _ := input["args"].(string)

	var cmd *exec.Cmd
	switch subcommand {
	case "diff":
		cmd = exec.CommandContext(ctx, "git", "diff")
	case "diff_staged":
		cmd = exec.CommandContext(ctx, "git", "diff", "--cached")
	case "status":
		cmd = exec.CommandContext(ctx, "git", "status", "--short")
	case "branch":
		cmd = exec.CommandContext(ctx, "git", "branch", "--show-current")
	case "checkout":
		parts := strings.Fields(args)
		if len(parts) == 0 {
			return ToolOutput{Success: false, Error: "args required for checkout"}
		}
		gitArgs := append([]string{"checkout"}, parts...)
		cmd = exec.CommandContext(ctx, "git", gitArgs...)
	case "checkout_new_branch":
		parts := strings.Fields(args)
		if len(parts) == 0 {
			return ToolOutput{Success: false, Error: "branch name required"}
		}
		gitArgs := append([]string{"checkout", "-b"}, parts...)
		cmd = exec.CommandContext(ctx, "git", gitArgs...)
	case "commit":
		msg, _ := input["message"].(string)
		if msg == "" {
			msg = args
		}
		cmd = exec.CommandContext(ctx, "git", "commit", "-m", msg)
	case "log":
		count := 10
		if v, ok := input["count"].(int); ok && v > 0 {
			count = v
		}
		cmd = exec.CommandContext(ctx, "git", "log", "--oneline", fmt.Sprintf("-%d", count)) //nolint:gosec // Git commands are safe
	case "add":
		parts := strings.Fields(args)
		if len(parts) == 0 {
			return ToolOutput{Success: false, Error: "args required for add"}
		}
		gitArgs := append([]string{"add"}, parts...)
		cmd = exec.CommandContext(ctx, "git", gitArgs...)
	default:
		return ToolOutput{Success: false, Error: fmt.Sprintf("unknown subcommand: %s", subcommand)}
	}

	if cmd != nil {
		cmd.Dir = t.RepoPath
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		return ToolOutput{
			Success: err == nil,
			Data: map[string]string{
				"stdout": strings.TrimSpace(stdout.String()),
				"stderr": strings.TrimSpace(stderr.String()),
			},
			Error: func() string {
				if err != nil {
					return err.Error()
				}
				return ""
			}(),
		}
	}

	return ToolOutput{Success: false, Error: "no command executed"}
}

// Diff returns the unstaged diff of the repository.
func (t *GitTool) Diff(ctx context.Context) (string, error) {
	result := t.Run(ctx, ToolInput{"subcommand": "diff"})
	if !result.Success {
		return "", fmt.Errorf("git diff failed: %s", result.Error)
	}
	data := result.Data.(map[string]string)
	return data["stdout"], nil
}

// CurrentBranch returns the name of the currently checked-out branch.
func (t *GitTool) CurrentBranch(ctx context.Context) (string, error) {
	result := t.Run(ctx, ToolInput{"subcommand": "branch"})
	if !result.Success {
		return "", fmt.Errorf("git branch failed: %s", result.Error)
	}
	data := result.Data.(map[string]string)
	return strings.TrimSpace(data["stdout"]), nil
}
