package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type DockerConfig struct {
	Image       string
	WorkDir     string
	MemoryLimit string
	CPULimit    string
	Network     string
	ExtraArgs   []string
}

type DockerSandbox struct {
	config DockerConfig
}

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

func (s *DockerSandbox) RunScript(ctx context.Context, script string, workDir string) (stdout, stderr string, err error) {
	return s.Run(ctx, script, workDir)
}

func (s *DockerSandbox) CheckAvailable() error {
	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker not available: %w", err)
	}
	return nil
}
