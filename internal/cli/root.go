// Package cli implements the command-line interface commands for AgentOS.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "agentos",
	Short: "AgentOS - Coding agent execution platform",
	Long: `AgentOS is a coding agent execution platform that uses LiteLLM
as the LLM gateway for safe, reproducible code generation.

Complete documentation is available at https://github.com/kazyamaz200/agentos`,
}

// Execute runs the root cobra command, parsing and dispatching CLI subcommands.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(reviewCmd)
}
