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
	"github.com/spf13/cobra"
)

var (
	prRepo  string
	prTitle string
	prBody  string
	prHead  string
	prBase  string
	prState string
)

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "GitHub Pull Request operations",
}

var prCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a Pull Request",
	Run: func(cmd *cobra.Command, args []string) {
		if err := createPR(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var prListCmd = &cobra.Command{
	Use:   "list",
	Short: "List Pull Requests",
	Run: func(cmd *cobra.Command, args []string) {
		if err := listPRs(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	prCmd.AddCommand(prCreateCmd)
	prCmd.AddCommand(prListCmd)
	rootCmd.AddCommand(prCmd)

	prCreateCmd.Flags().StringVarP(&prRepo, "repo", "r", "", "Repository (owner/name)")
	prCreateCmd.Flags().StringVarP(&prTitle, "title", "t", "", "PR title")
	prCreateCmd.Flags().StringVarP(&prBody, "body", "b", "", "PR body (or path to file)")
	prCreateCmd.Flags().StringVarP(&prHead, "head", "H", "", "Head branch")
	prCreateCmd.Flags().StringVarP(&prBase, "base", "B", "main", "Base branch")
	_ = prCreateCmd.MarkFlagRequired("repo")  //nolint:errcheck // cobra returns error only for invalid flag name
	_ = prCreateCmd.MarkFlagRequired("title") //nolint:errcheck // cobra returns error only for invalid flag name
	_ = prCreateCmd.MarkFlagRequired("head")  //nolint:errcheck // cobra returns error only for invalid flag name

	prListCmd.Flags().StringVarP(&prRepo, "repo", "r", "", "Repository (owner/name)")
	prListCmd.Flags().StringVarP(&prState, "state", "s", "open", "PR state (open/closed/all)")
	_ = prListCmd.MarkFlagRequired("repo") //nolint:errcheck // cobra returns error only for invalid flag name
}

func createPR() error {
	owner, name, err := parseRepo(prRepo)
	if err != nil {
		return err
	}

	body := prBody
	if body != "" {
		if _, statErr := os.Stat(body); statErr == nil {
			data, readErr := os.ReadFile(body)
			if readErr == nil {
				body = string(data)
			}
		}
	}

	client := github.NewClient(owner, name)
	pr, err := client.CreatePR(github.CreatePRRequest{
		Title: prTitle,
		Body:  body,
		Head:  prHead,
		Base:  prBase,
	})
	if err != nil {
		return fmt.Errorf("create PR: %w", err)
	}

	fmt.Printf("Pull Request created: #%d\n", pr.Number)
	fmt.Printf("URL: %s\n", pr.HTMLURL)
	return nil
}

func listPRs() error {
	owner, name, err := parseRepo(prRepo)
	if err != nil {
		return err
	}

	client := github.NewClient(owner, name)
	prs, err := client.ListPRs(prState)
	if err != nil {
		return fmt.Errorf("list PRs: %w", err)
	}

	if len(prs) == 0 {
		fmt.Println("No pull requests found.")
		return nil
	}

	for _, pr := range prs {
		fmt.Printf("#%d [%s] %s (%s -> %s)\n", pr.Number, pr.State, pr.Title, pr.Head, pr.Base)
	}
	return nil
}
