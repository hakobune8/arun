package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// DockerConfig specifies the image, resource limits, network mode, and extra
// arguments for a Docker sandbox container.
type DockerConfig struct {
	Image       string
	WorkDir     string
	MemoryLimit string
	CPULimit    string
	Network     string
	ExtraArgs   []string
}

// DockerSandbox runs commands inside a Docker container with configurable
// resource limits and network isolation.
type DockerSandbox struct {
	config DockerConfig
}

// NewDockerSandbox returns a DockerSandbox with sensible defaults (golang:1.22-alpine,
// 1 GB memory, 1.0 CPU, no network).
func NewDockerSandbox(config DockerConfig) *DockerSandbox {
	if config.Image == "" {
		config.Image = "golang:1.22-alpine"
	}
	if config.MemoryLimit == "" {
		config.MemoryLimit = "1g"
	}
	if config.CPULimit == "" {
		config.CPULimit = "1.0"
	}
	if config.Network == "" {
		config.Network = "none"
	}
	return &DockerSandbox{config: config}
}

// Run executes command inside a Docker container. If workDir is non-empty it
// is mounted at /workspace.
func (s *DockerSandbox) Run(ctx context.Context, command string, workDir string) (stdout, stderr string, err error) {
	args := []string{"run", "--rm"}

	if s.config.MemoryLimit != "" {
		args = append(args, "--memory", s.config.MemoryLimit)
	}
	if s.config.CPULimit != "" {
		args = append(args, "--cpus", s.config.CPULimit)
	}
	if s.config.Network != "" {
		args = append(args, "--network", s.config.Network)
	}
	if workDir != "" {
		args = append(args, "-v", workDir+":/workspace", "-w", "/workspace")
	}
	args = append(args, s.config.ExtraArgs...)
	args = append(args, s.config.Image, "sh", "-c", command)

	cmd := exec.CommandContext(ctx, "docker", args...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err = cmd.Run()
	return strings.TrimSpace(stdoutBuf.String()), strings.TrimSpace(stderrBuf.String()), err
}

// RunScript is a convenience wrapper around Run that passes script directly
// as the command.
func (s *DockerSandbox) RunScript(ctx context.Context, script string, workDir string) (stdout, stderr string, err error) {
	return s.Run(ctx, script, workDir)
}

// CheckAvailable verifies that Docker is installed and running by executing
// docker info.
func (s *DockerSandbox) CheckAvailable() error {
	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker not available: %w", err)
	}
	return nil
}
