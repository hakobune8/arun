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
	checksListCmd.MarkFlagRequired("repo")
	checksListCmd.MarkFlagRequired("ref")
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

	for _, cr := range checkRuns {
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
