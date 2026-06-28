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
