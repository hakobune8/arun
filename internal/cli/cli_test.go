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
	"testing"
)

func TestRootCommand_HasUse(t *testing.T) {
	if rootCmd.Use != "agentos" {
		t.Errorf("rootCmd.Use = %q, want %q", rootCmd.Use, "agentos")
	}
}

func TestRootCommand_HasSubcommands(t *testing.T) {
	expected := []string{
		"version", "run", "review", "issue", "pr", "checks",
		"ci-fix", "search", "memory", "mcp", "serve", "agent",
		"orchestrate", "guideline",
	}
	for _, name := range expected {
		cmd, _, err := rootCmd.Find([]string{name})
		if err != nil {
			t.Errorf("expected subcommand %q to be registered, got error: %v", name, err)
			continue
		}
		if cmd.Name() != name {
			t.Errorf("subcommand %q has Name = %q", name, cmd.Name())
		}
	}
}

func TestRunCommand_HasDefinitionFlag(t *testing.T) {
	flag := runCmd.Flags().Lookup("definition")
	if flag == nil {
		t.Fatal("expected run command to expose --definition")
	}
}
