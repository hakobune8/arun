package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	agentosgh "github.com/kazyamaz200/agentos/internal/github"
	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/profile"
	"github.com/kazyamaz200/agentos/internal/runtime"
	"github.com/kazyamaz200/agentos/internal/sandbox"
	"github.com/kazyamaz200/agentos/internal/task"
	"github.com/spf13/cobra"
)

var (
	taskFile       string
	profileFile    string
	dryRun         bool
	verbose        bool
	runCreatePR    bool
	runPRRepo      string
)

var runCmd = &cobra.Command{
	Use:   "run --task <task.yaml> --profile <profile.yaml>",
	Short: "Run a coding task",
	Long: `Run a coding task with AgentOS.
Reads a task YAML and profile YAML, plans the implementation,
executes the plan against the target repository, and produces a patch.

Example:
  agentos run --task examples/task.issue.yaml --profile profiles/go_backend.yaml`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runTask(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	runCmd.Flags().StringVar(&taskFile, "task", "", "Path to task YAML file")
	runCmd.Flags().StringVar(&profileFile, "profile", "", "Path to profile YAML file")
	runCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview actions without making changes")
	runCmd.Flags().BoolVar(&verbose, "verbose", false, "Enable verbose output")
	runCmd.Flags().BoolVar(&runCreatePR, "pr", false, "Create a PR after successful run")
	runCmd.Flags().StringVar(&runPRRepo, "pr-repo", "", "GitHub repo for PR (owner/name)")
	runCmd.MarkFlagRequired("task")
	runCmd.MarkFlagRequired("profile")
}

func runTask() error {
	tk, err := task.Load(taskFile)
	if err != nil {
		return fmt.Errorf("load task: %w", err)
	}

	prof, err := profile.Load(profileFile)
	if err != nil {
		return fmt.Errorf("load profile: %w", err)
	}

	repoPath := tk.Repo
	if !filepath.IsAbs(repoPath) {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get cwd: %w", err)
		}
		repoPath = filepath.Join(cwd, repoPath)
	}

	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return fmt.Errorf("repository path does not exist: %s", repoPath)
	}

	llmConfig := llm.DefaultConfig()
	llmClient := llm.NewLiteLLMClient(llmConfig)

	if verbose {
		fmt.Printf("Task: %s\n", tk.ID)
		fmt.Printf("Title: %s\n", tk.Title)
		fmt.Printf("Profile: %s\n", prof.Name)
		fmt.Printf("Repo: %s\n", repoPath)
		fmt.Printf("LLM: %s\n", llmConfig.BaseURL)
	}

	ws := sandbox.NewWorkspace(repoPath)
	cfg := &runtime.Config{
		DryRun:  dryRun,
		Verbose: verbose,
	}

	rt := runtime.NewRuntime(llmClient, prof, ws, cfg)
	if err := rt.Run(context.Background(), tk); err != nil {
		return err
	}

	if runCreatePR {
		if runPRRepo == "" {
			return fmt.Errorf("--pr-repo is required when --pr is set")
		}
		if err := createPRFromRun(tk); err != nil {
			return fmt.Errorf("create PR: %w", err)
		}
	}

	return nil
}

func createPRFromRun(tk *task.Task) error {
	owner, name, err := parseRepo(runPRRepo)
	if err != nil {
		return err
	}

	prBody := readRunArtifact(tk, "pr_body.md")
	if prBody == "" {
		prBody = tk.Description
	}

	client := agentosgh.NewClient(owner, name)
	pr, err := client.CreatePR(agentosgh.CreatePRRequest{
		Title: tk.Title,
		Body:  prBody,
		Head:  tk.Branch,
		Base:  tk.BaseBranch,
	})
	if err != nil {
		return fmt.Errorf("create PR: %w", err)
	}

	fmt.Printf("Pull Request created: #%d\n", pr.Number)
	fmt.Printf("URL: %s\n", pr.HTMLURL)
	return nil
}

func readRunArtifact(tk *task.Task, name string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(homeDir, ".agentos", "runs", tk.ID, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
