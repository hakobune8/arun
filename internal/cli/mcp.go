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
	"context"
	"fmt"
	"os"

	"github.com/kazyamaz200/agentos/internal/mcp"
	"github.com/spf13/cobra"
)

var (
	mcpCommand string
	mcpTool    string
	mcpArgs    []string
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "MCP (Model Context Protocol) operations",
}

var mcpConnectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Connect to an MCP server and list tools",
	Run: func(cmd *cobra.Command, args []string) {
		if err := runMCPConnect(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var mcpCallCmd = &cobra.Command{
	Use:   "call",
	Short: "Call an MCP tool",
	Run: func(cmd *cobra.Command, args []string) {
		if err := runMCPCall(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	mcpCmd.AddCommand(mcpConnectCmd)
	mcpCmd.AddCommand(mcpCallCmd)
	rootCmd.AddCommand(mcpCmd)

	mcpConnectCmd.Flags().StringVarP(&mcpCommand, "command", "c", "", "MCP server command (e.g., npx @anthropic/mcp-serve)")
	_ = mcpConnectCmd.MarkFlagRequired("command") //nolint:errcheck // cobra returns error only for invalid flag name

	mcpCallCmd.Flags().StringVarP(&mcpCommand, "command", "c", "", "MCP server command")
	mcpCallCmd.Flags().StringVarP(&mcpTool, "tool", "t", "", "Tool name to call")
	mcpCallCmd.Flags().StringArrayVar(&mcpArgs, "arg", nil, "Arguments (key=value)")
	_ = mcpCallCmd.MarkFlagRequired("command") //nolint:errcheck // cobra returns error only for invalid flag name
	_ = mcpCallCmd.MarkFlagRequired("tool")    //nolint:errcheck // cobra returns error only for invalid flag name
}

func runMCPConnect() error {
	parts := splitCommand(mcpCommand)
	if len(parts) == 0 {
		return fmt.Errorf("invalid command")
	}

	client := mcp.NewClient(parts[0], parts[1:]...)
	if err := client.Connect(context.Background()); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Close()

	info := client.Info()
	if info != nil {
		fmt.Printf("Connected to: %s v%s\n", info.ServerInfo.Name, info.ServerInfo.Version)
	}

	tools, err := client.ListTools(context.Background())
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}

	fmt.Printf("\nAvailable tools (%d):\n", len(tools))
	for _, t := range tools {
		fmt.Printf("  - %s: %s\n", t.Name, t.Description)
	}

	return nil
}

func runMCPCall() error {
	parts := splitCommand(mcpCommand)
	if len(parts) == 0 {
		return fmt.Errorf("invalid command")
	}

	client := mcp.NewClient(parts[0], parts[1:]...)
	if err := client.Connect(context.Background()); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Close()

	args := make(map[string]interface{})
	for _, a := range mcpArgs {
		eq := -1
		for i, c := range a {
			if c == '=' {
				eq = i
				break
			}
		}
		if eq > 0 {
			args[a[:eq]] = a[eq+1:]
		}
	}

	result, err := client.CallTool(context.Background(), mcpTool, args)
	if err != nil {
		return fmt.Errorf("call tool: %w", err)
	}

	for _, c := range result.Content {
		fmt.Println(c.Text)
	}

	return nil
}

func splitCommand(cmd string) []string {
	var parts []string
	current := ""
	inQuote := false
	for _, c := range cmd {
		if c == '"' || c == '\'' {
			inQuote = !inQuote
			continue
		}
		if c == ' ' && !inQuote {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
			continue
		}
		current += string(c)
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}
