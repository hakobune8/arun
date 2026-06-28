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
	prCreateCmd.MarkFlagRequired("repo")
	prCreateCmd.MarkFlagRequired("title")
	prCreateCmd.MarkFlagRequired("head")

	prListCmd.Flags().StringVarP(&prRepo, "repo", "r", "", "Repository (owner/name)")
	prListCmd.Flags().StringVarP(&prState, "state", "s", "open", "PR state (open/closed/all)")
	prListCmd.MarkFlagRequired("repo")
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
