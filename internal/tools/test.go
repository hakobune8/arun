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

func (t *TestTool) Run(ctx context.Context, input ToolInput) ToolOutput {
	command, _ := input["command"].(string)
	if command == "" {
		command = "go test ./..."
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = t.WorkDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := ToolOutput{
		Success: err == nil,
		Data: map[string]string{
			"stdout": strings.TrimSpace(stdout.String()),
			"stderr": strings.TrimSpace(stderr.String()),
			"command": command,
		},
	}

	if err != nil {
		output.Error = fmt.Sprintf("test command failed: %v", err)
	}

	return output
}
