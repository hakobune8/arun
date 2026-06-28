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
	checksRepo string
	checksRef  string
)

var checksCmd = &cobra.Command{
	Use:   "checks",
	Short: "GitHub CI check operations",
}

var checksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List CI check runs for a ref",
	Run: func(cmd *cobra.Command, args []string) {
		if err := listChecks(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	checksCmd.AddCommand(checksListCmd)
	rootCmd.AddCommand(checksCmd)

	checksListCmd.Flags().StringVarP(&checksRepo, "repo", "r", "", "Repository (owner/name)")
	checksListCmd.Flags().StringVarP(&checksRef, "ref", "", "", "Git ref (branch, SHA)")
	_ = checksListCmd.MarkFlagRequired("repo") //nolint:errcheck // cobra returns error only for invalid flag name
	_ = checksListCmd.MarkFlagRequired("ref")   //nolint:errcheck // cobra returns error only for invalid flag name
}

func listChecks() error {
	owner, name, err := parseRepo(checksRepo)
	if err != nil {
		return err
	}

	client := github.NewClient(owner, name)
	checkRuns, err := client.GetCheckRuns(checksRef)
	if err != nil {
		return fmt.Errorf("get check runs: %w", err)
	}

	if len(checkRuns) == 0 {
		fmt.Println("No check runs found.")
		return nil
	}

	for i := range checkRuns {
		cr := checkRuns[i]
		status := cr.Status
		if cr.Conclusion != "" {
			status = cr.Conclusion
		}
		fmt.Printf("- %s: %s", cr.Name, status)
		if cr.HTMLURL != "" {
			fmt.Printf(" (%s)", cr.HTMLURL)
		}
		fmt.Println()
	}

	return nil
}
