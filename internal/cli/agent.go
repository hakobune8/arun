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

package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/kazyamaz200/agentos/internal/agent"
	"github.com/kazyamaz200/agentos/internal/factory"
	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Agent management operations",
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered agents",
	Run: func(cmd *cobra.Command, args []string) {
		if err := runAgentList(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var agentCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an agent from a template file",
	Long: `Create runnable agent definitions from a template file.

Example:
  agentos agent create --template profiles/agents/template.yaml`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runAgentCreate(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var agentRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run an agent with a task description",
	Long: `Run an agent from a template with an inline task description.

Example:
  agentos agent run \
    --template profiles/agents/template.yaml \
    --agent coder \
    --task "Add validation and tests"`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runAgentRun(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var (
	agentTemplate string
	agentName     string
	agentTask     string
)

func init() {
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentCreateCmd)
	agentCmd.AddCommand(agentRunCmd)
	rootCmd.AddCommand(agentCmd)

	agentCreateCmd.Flags().StringVarP(&agentTemplate, "template", "t", "profiles/agents/template.yaml", "Agent template file")

	agentRunCmd.Flags().StringVarP(&agentTemplate, "template", "t", "profiles/agents/template.yaml", "Agent template file")
	agentRunCmd.Flags().StringVarP(&agentName, "agent", "a", "", "Agent name to run")
	agentRunCmd.Flags().StringVarP(&agentTask, "task", "", "", "Task description")
	_ = agentRunCmd.MarkFlagRequired("agent") //nolint:errcheck // cobra returns error only for invalid flag name
	_ = agentRunCmd.MarkFlagRequired("task")  //nolint:errcheck // cobra returns error only for invalid flag name
}

func runAgentList() error {
	reg := agent.DefaultRegistry()
	agents := reg.List()

	if len(agents) == 0 {
		fmt.Println("No agents registered.")
		return nil
	}

	fmt.Println("Registered agents:")
	for i := range agents {
		info := &agents[i]
		fmt.Printf("  %s v%s\n", info.Name, info.Version)
		fmt.Printf("    %s\n", info.Description)
		if len(info.RequiredTools) > 0 {
			fmt.Printf("    Tools: %v\n", info.RequiredTools)
		}
		fmt.Println()
	}
	return nil
}

func runAgentCreate() error {
	wd, _ := os.Getwd()
	f := factory.NewFactory(wd)

	agents, err := f.CreateAgentsFromFile(agentTemplate)
	if err != nil {
		return fmt.Errorf("create agents: %w", err)
	}

	fmt.Printf("Created %d agent(s) from %s\n", len(agents), agentTemplate)
	for _, a := range agents {
		fmt.Printf("  - %s (role: %s, model: %s)\n", a.Def.Name, a.Def.Role, a.Def.Model)
		fmt.Printf("    Tools: %v\n", a.Def.Tools)
	}
	return nil
}

func runAgentRun() error {
	wd, _ := os.Getwd()
	f := factory.NewFactory(wd)
	runner := factory.NewAgentRunner(f)

	def := &factory.AgentDef{
		Name: agentName,
	}

	return runner.RunAgent(context.Background(), def, agentTask)
}
