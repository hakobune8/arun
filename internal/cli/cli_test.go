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
	"testing"
	"time"
)

func TestRootCommand_HasUse(t *testing.T) {
	if rootCmd.Use != "arun" {
		t.Errorf("rootCmd.Use = %q, want %q", rootCmd.Use, "arun")
	}
}

func TestRootCommand_HasSubcommands(t *testing.T) {
	expected := []string{
		"version", "run", "review", "issue", "pr", "checks",
		"ci-fix", "search", "memory", "mcp", "serve", "agent",
		"orchestrate", "guideline", "completion",
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

func TestCompletionCommand_GeneratesBash(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"completion", "bash"})
	if err != nil {
		t.Fatalf("find completion command: %v", err)
	}
	if cmd.Name() != "completion" {
		t.Fatalf("command = %q, want completion", cmd.Name())
	}
}

func TestRunCommand_HasDefinitionFlag(t *testing.T) {
	flag := runCmd.Flags().Lookup("definition")
	if flag == nil {
		t.Fatal("expected run command to expose --definition")
	}
}

func TestParseOrchestrateSubtaskTimeout(t *testing.T) {
	t.Setenv("ARUN_ORCHESTRATE_SUBTASK_TIMEOUT", "")
	got, err := parseOrchestrateSubtaskTimeout("5m")
	if err != nil {
		t.Fatalf("parseOrchestrateSubtaskTimeout() error = %v", err)
	}
	if got != 5*time.Minute {
		t.Fatalf("timeout = %s, want 5m", got)
	}

	t.Setenv("ARUN_ORCHESTRATE_SUBTASK_TIMEOUT", "2m")
	got, err = parseOrchestrateSubtaskTimeout("")
	if err != nil {
		t.Fatalf("parseOrchestrateSubtaskTimeout(env) error = %v", err)
	}
	if got != 2*time.Minute {
		t.Fatalf("env timeout = %s, want 2m", got)
	}

	if _, err := parseOrchestrateSubtaskTimeout("bad"); err == nil {
		t.Fatal("parseOrchestrateSubtaskTimeout(bad) succeeded, want error")
	}
}
