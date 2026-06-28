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
	cifixCmd.MarkFlagRequired("repo")
	cifixCmd.MarkFlagRequired("ref")
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
