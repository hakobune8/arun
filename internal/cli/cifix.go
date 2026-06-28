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

// Package cli implements the command-line interface commands for AgentOS.
package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/kazyamaz200/agentos/internal/agent"
	"github.com/kazyamaz200/agentos/internal/github"
	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/spf13/cobra"
)

var (
	cifixRepo string
	cifixRef  string
)

var cifixCmd = &cobra.Command{
	Use:   "ci-fix",
	Short: "Analyze and fix CI failures",
	Long: `Fetch CI check results for a given ref, analyze failures using LLM,
and suggest fixes.

Example:
  agentos ci-fix --repo owner/repo --ref main`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runCIFix(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(cifixCmd)
	cifixCmd.Flags().StringVarP(&cifixRepo, "repo", "r", "", "Repository (owner/name)")
	cifixCmd.Flags().StringVarP(&cifixRef, "ref", "", "", "Git ref (branch, SHA)")
	_ = cifixCmd.MarkFlagRequired("repo") //nolint:errcheck // cobra returns error only for invalid flag name
	_ = cifixCmd.MarkFlagRequired("ref")   //nolint:errcheck // cobra returns error only for invalid flag name
}

func runCIFix() error {
	owner, name, err := parseRepo(cifixRepo)
	if err != nil {
		return err
	}

	llmConfig := llm.DefaultConfig()
	llmClient := llm.NewLiteLLMClient(llmConfig)
	ghClient := github.NewClient(owner, name)

	fixer := agent.NewCIFixer(llmClient, ghClient)
	result, err := fixer.AnalyzeAndFix(context.Background(), cifixRef)
	if err != nil {
		return fmt.Errorf("CI fix failed: %w", err)
	}

	if result.Success && len(result.FailedChecks) == 0 {
		fmt.Println("All CI checks passed!")
		return nil
	}

	fmt.Printf("Found %d failed checks.\n", len(result.FailedChecks))
	for _, fc := range result.FailedChecks {
		fmt.Printf("  - %s (%s)\n", fc.Name, fc.Conclusion)
	}

	if result.FixSummary != "" {
		fmt.Printf("\nFix suggestion:\n%s\n", result.FixSummary)
	}

	if result.Error != "" {
		fmt.Fprintf(os.Stderr, "Error during fix: %s\n", result.Error)
	}

	return nil
}
