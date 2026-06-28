// Package cli implements the command-line interface commands for AgentOS.
package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/kazyamaz200/agentos/internal/agent"
	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/profile"
	"github.com/kazyamaz200/agentos/internal/runtime"
	"github.com/kazyamaz200/agentos/internal/sandbox"
	"github.com/kazyamaz200/agentos/internal/task"
	"github.com/spf13/cobra"
)

var (
	reviewRepo    string
	reviewProfile string
)

var reviewCmd = &cobra.Command{
	Use:   "review --repo <path> --profile <profile.yaml>",
	Short: "Review code changes in a repository",
	Long: `Review code changes in a repository using an LLM.
Generates a review summary based on the current diff.

Example:
  agentos review --repo ./repo --profile profiles/reviewer.yaml`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runReview(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	reviewCmd.Flags().StringVar(&reviewRepo, "repo", "", "Path to the repository")
	reviewCmd.Flags().StringVar(&reviewProfile, "profile", "", "Path to profile YAML file")
	reviewCmd.MarkFlagRequired("repo")
	reviewCmd.MarkFlagRequired("profile")
}

func runReview() error {
	prof, err := profile.Load(reviewProfile)
	if err != nil {
		return fmt.Errorf("load profile: %w", err)
	}

	if _, err := os.Stat(reviewRepo); os.IsNotExist(err) {
		return fmt.Errorf("repository path does not exist: %s", reviewRepo)
	}

	llmConfig := llm.DefaultConfig()
	llmClient := llm.NewLiteLLMClient(llmConfig)

	ws := sandbox.NewWorkspace(reviewRepo)
	cfg := &runtime.Config{Verbose: true}

	planner := agent.NewPlanner(llmClient)
	rt := runtime.NewRuntime(llmClient, prof, ws, cfg, planner)

	mockTask := &task.Task{
		ID:   "review-" + reviewRepo,
		Type: "review",
		Repo: reviewRepo,
	}

	return rt.Run(context.Background(), mockTask)
}
