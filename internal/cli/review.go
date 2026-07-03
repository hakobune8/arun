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

// Package cli implements the command-line interface commands for ARUN.
package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/hakobune8/arun/internal/agent"
	"github.com/hakobune8/arun/internal/llm"
	"github.com/hakobune8/arun/internal/profile"
	"github.com/hakobune8/arun/internal/runtime"
	"github.com/hakobune8/arun/internal/sandbox"
	"github.com/hakobune8/arun/internal/task"
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
  arun review --repo ./repo --profile profiles/reviewer.yaml`,
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
	_ = reviewCmd.MarkFlagRequired("repo")    //nolint:errcheck // cobra returns error only for invalid flag name
	_ = reviewCmd.MarkFlagRequired("profile") //nolint:errcheck // cobra returns error only for invalid flag name
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

	agt := agent.NewBaseAgent(prof.Name, llmClient)
	rt := runtime.NewRuntime(llmClient, prof, ws, cfg, agt)

	mockTask := &task.Task{
		ID:   "review-" + reviewRepo,
		Type: "review",
		Repo: reviewRepo,
	}

	return rt.Run(context.Background(), mockTask)
}
