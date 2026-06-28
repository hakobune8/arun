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
	"fmt"
	"os"

	"github.com/kazyamaz200/agentos/internal/github"
	"github.com/kazyamaz200/agentos/internal/task"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	issueRepo  string
	issueState string
)

var issueCmd = &cobra.Command{
	Use:   "issue",
	Short: "GitHub Issue operations",
}

var issueListCmd = &cobra.Command{
	Use:   "list",
	Short: "List GitHub Issues",
	Run: func(cmd *cobra.Command, args []string) {
		if err := listIssues(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var issueFetchCmd = &cobra.Command{
	Use:   "fetch <number>",
	Short: "Fetch a GitHub Issue as a task",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := fetchIssue(args[0]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	issueCmd.AddCommand(issueListCmd)
	issueCmd.AddCommand(issueFetchCmd)
	rootCmd.AddCommand(issueCmd)

	issueListCmd.Flags().StringVarP(&issueRepo, "repo", "r", "", "Repository (owner/name)")
	issueListCmd.Flags().StringVarP(&issueState, "state", "s", "open", "Issue state (open/closed/all)")
	_ = issueListCmd.MarkFlagRequired("repo")  //nolint:errcheck // cobra returns error only for invalid flag name

	issueFetchCmd.Flags().StringVarP(&issueRepo, "repo", "r", "", "Repository (owner/name)")
	_ = issueFetchCmd.MarkFlagRequired("repo") //nolint:errcheck // cobra returns error only for invalid flag name
}

func parseRepo(repo string) (owner, name string, err error) {
	if repo == "" {
		return "", "", fmt.Errorf("repo is required (format: owner/name)")
	}
	parts := splitRepo(repo)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repo format: %s (expected owner/name)", repo)
	}
	return parts[0], parts[1], nil
}

func splitRepo(repo string) []string {
	var parts []string
	current := ""
	for _, c := range repo {
		if c == '/' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

func listIssues() error {
	owner, name, err := parseRepo(issueRepo)
	if err != nil {
		return err
	}

	client := github.NewClient(owner, name)
	issues, err := client.ListIssues(issueState)
	if err != nil {
		return fmt.Errorf("list issues: %w", err)
	}

	if len(issues) == 0 {
		fmt.Println("No issues found.")
		return nil
	}

	for _, issue := range issues {
		fmt.Printf("#%d [%s] %s\n", issue.Number, issue.State, issue.Title)
	}
	return nil
}

func fetchIssue(numberStr string) error {
	owner, name, err := parseRepo(issueRepo)
	if err != nil {
		return err
	}

	var number int
	if _, err := fmt.Sscanf(numberStr, "%d", &number); err != nil {
		return fmt.Errorf("invalid issue number: %s", numberStr)
	}

	client := github.NewClient(owner, name)
	issue, err := client.GetIssue(number)
	if err != nil {
		return fmt.Errorf("get issue: %w", err)
	}

	repoPath := fmt.Sprintf("https://github.com/%s/%s.git", owner, name)
	t := task.FromGitHubIssue(issue, repoPath)

	taskDir := ".agentos/tasks"
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		return fmt.Errorf("create task dir: %w", err)
	}

	taskFile := fmt.Sprintf("%s/issue-%d.yaml", taskDir, number)
	data, err := yaml.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}
	if err := os.WriteFile(taskFile, data, 0o600); err != nil {
		return fmt.Errorf("write task: %w", err)
	}

	fmt.Printf("Task saved to %s\n", taskFile)
	fmt.Printf("  ID: %s\n", t.ID)
	fmt.Printf("  Title: %s\n", t.Title)
	fmt.Printf("  Branch: %s\n", t.Branch)

	return nil
}
