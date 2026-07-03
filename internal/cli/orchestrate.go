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

package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hakobune8/arun/internal/agent"
	"github.com/hakobune8/arun/internal/llm"
	"github.com/hakobune8/arun/internal/orchestrator"
	"github.com/hakobune8/arun/internal/runtime"
	"github.com/hakobune8/arun/internal/sandbox"
	"github.com/spf13/cobra"
)

var orchestrateCmd = &cobra.Command{
	Use:   "orchestrate",
	Short: "Run multi-agent orchestration",
	Long: `Coordinate multiple agents to work on a complex task.
Agents are selected from the registry and can work sequentially or in parallel.

Example:
  arun orchestrate \
    --agents "go-backend,reviewer,docs" \
    --strategy parallel \
    --repo . \
    --task "Implement user authentication, tests, and docs"`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runOrchestrate(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var (
	orchAgents         string
	orchTask           string
	orchStrategy       string
	orchRepo           string
	orchSubtaskTimeout string
)

func init() {
	rootCmd.AddCommand(orchestrateCmd)
	orchestrateCmd.Flags().StringVarP(&orchAgents, "agents", "a", "go-backend", "Comma-separated agent names from registry")
	orchestrateCmd.Flags().StringVarP(&orchTask, "task", "", "", "Task description")
	orchestrateCmd.Flags().StringVarP(&orchStrategy, "strategy", "s", "sequential", "Coordination strategy (sequential/parallel)")
	orchestrateCmd.Flags().StringVarP(&orchRepo, "repo", "r", ".", "Repository path")
	orchestrateCmd.Flags().StringVar(&orchSubtaskTimeout, "subtask-timeout", "", "Maximum runtime for each subtask, e.g. 5m. Defaults to ARUN_ORCHESTRATE_SUBTASK_TIMEOUT when set")
	_ = orchestrateCmd.MarkFlagRequired("task") //nolint:errcheck // cobra returns error only for invalid flag name
}

func runOrchestrate() error {
	llmCfg := llm.DefaultConfig()
	llmClient := llm.NewLiteLLMClient(llmCfg)

	ws := sandbox.NewWorkspace(orchRepo)
	cfg := &runtime.Config{Verbose: true}

	reg := agent.DefaultRegistry()
	agentNames := splitComma(orchAgents)
	agents := make(map[string]runtime.Agent)

	for _, name := range agentNames {
		a, err := reg.Create(name, llmClient)
		if err != nil {
			return fmt.Errorf("lookup agent %q: %w", name, err)
		}
		agents[name] = a
	}

	orch := orchestrator.NewOrchestrator(llmClient, ws, agents, cfg)
	if orchStrategy == "parallel" {
		orch.SetStrategy(orchestrator.StrategyParallel)
	}
	subtaskTimeout, err := parseOrchestrateSubtaskTimeout(orchSubtaskTimeout)
	if err != nil {
		return err
	}
	if subtaskTimeout > 0 {
		orch.SetSubtaskTimeout(subtaskTimeout)
	}

	fmt.Printf("Orchestrating %d agents: %v\n", len(agents), agentNames)
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
	if err := os.WriteFile(outputFile, []byte(summary), 0o600); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	fmt.Printf("Result saved to %s\n", outputFile)

	return nil
}

func parseOrchestrateSubtaskTimeout(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("ARUN_ORCHESTRATE_SUBTASK_TIMEOUT"))
	}
	if raw == "" {
		return 0, nil
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid subtask timeout %q: %w", raw, err)
	}
	if timeout < 0 {
		return 0, fmt.Errorf("subtask timeout must be non-negative")
	}
	return timeout, nil
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			if i > start {
				result = append(result, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}
