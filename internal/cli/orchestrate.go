// Package cli implements the command-line interface commands for AgentOS.
package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/kazyamaz200/agentos/internal/factory"
	"github.com/kazyamaz200/agentos/internal/orchestrator"
	"github.com/spf13/cobra"
)

var orchestrateCmd = &cobra.Command{
	Use:   "orchestrate",
	Short: "Run multi-agent orchestration",
	Long: `Coordinate multiple agents to work on a complex task.
Agents are defined in a template file and can work sequentially or in parallel.

Example:
  agentos orchestrate --template profiles/agents/template.yaml --task "Implement user authentication"`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runOrchestrate(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var (
	orchTemplate string
	orchTask     string
	orchStrategy string
)

func init() {
	rootCmd.AddCommand(orchestrateCmd)
	orchestrateCmd.Flags().StringVarP(&orchTemplate, "template", "t", "profiles/agents/template.yaml", "Agent template file")
	orchestrateCmd.Flags().StringVarP(&orchTask, "task", "", "", "Task description")
	orchestrateCmd.Flags().StringVarP(&orchStrategy, "strategy", "s", "sequential", "Coordination strategy (sequential/parallel)")
	orchestrateCmd.MarkFlagRequired("task")
}

func runOrchestrate() error {
	wd, _ := os.Getwd()
	f := factory.NewFactory(wd)

	tmpl, err := factory.LoadTemplate(orchTemplate)
	if err != nil {
		return fmt.Errorf("load template: %w", err)
	}

	agents, err := f.CreateAgentsFromTemplate(tmpl)
	if err != nil {
		return fmt.Errorf("create agents: %w", err)
	}

	orch := orchestrator.NewOrchestrator(f, agents)
	if orchStrategy == "parallel" {
		orch.SetStrategy(orchestrator.StrategyParallel)
	}

	fmt.Printf("Orchestrating %d agents\n", len(agents))
	for _, a := range agents {
		fmt.Printf("  - %s (%s)\n", a.Def.Name, a.Def.Role)
	}
	fmt.Printf("Strategy: %s\n\n", orchStrategy)

	plan, err := orch.Plan(context.Background(), orchTask)
	if err != nil {
		return fmt.Errorf("plan: %w", err)
	}

	fmt.Printf("Plan: %s\n", plan.Description)
	fmt.Printf("Subtasks: %d\n\n", len(plan.Subtasks))
	for _, s := range plan.Subtasks {
		fmt.Printf("  [%s] %s (assigned to: %s)\n", s.ID, s.Description, s.AgentName)
	}

	fmt.Println("\nExecuting...")
	results, err := orch.Execute(context.Background(), plan)
	if err != nil {
		return fmt.Errorf("execute: %w", err)
	}

	summary := orch.MergeResults(results)
	fmt.Println(summary)

	outputFile := "orchestration_result.md"
	if err := os.WriteFile(outputFile, []byte(summary), 0644); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	fmt.Printf("Result saved to %s\n", outputFile)

	return nil
}
