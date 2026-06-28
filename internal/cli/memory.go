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

	"github.com/kazyamaz200/agentos/internal/embedding"
	"github.com/kazyamaz200/agentos/internal/memory"
	"github.com/spf13/cobra"
)

var memCmd = &cobra.Command{
	Use:   "memory",
	Short: "Agent memory operations",
}

var memSaveCmd = &cobra.Command{
	Use:   "save",
	Short: "Save content to agent memory",
	Run: func(cmd *cobra.Command, args []string) {
		if err := runMemSave(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var memSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search agent memory",
	Run: func(cmd *cobra.Command, args []string) {
		if err := runMemSearch(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var memClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear all agent memory",
	Run: func(cmd *cobra.Command, args []string) {
		if err := runMemClear(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var (
	memContent   string
	memType      string
	memSearchQ   string
	memLimit     int
)

func init() {
	memCmd.AddCommand(memSaveCmd)
	memCmd.AddCommand(memSearchCmd)
	memCmd.AddCommand(memClearCmd)
	rootCmd.AddCommand(memCmd)

	memSaveCmd.Flags().StringVarP(&memContent, "content", "c", "", "Content to save")
	memSaveCmd.Flags().StringVarP(&memType, "type", "t", "note", "Entry type")
	_ = memSaveCmd.MarkFlagRequired("content") //nolint:errcheck // cobra returns error only for invalid flag name

	memSearchCmd.Flags().StringVarP(&memSearchQ, "query", "q", "", "Search query")
	memSearchCmd.Flags().IntVarP(&memLimit, "limit", "l", 10, "Max results")
	_ = memSearchCmd.MarkFlagRequired("query") //nolint:errcheck // cobra returns error only for invalid flag name
}

func runMemSave() error {
	vs := newVectorStore()
	emb := embedding.NewLiteLLMEmbedder()
	store := memory.NewMemoryStore(vs, emb)

	entry := memory.Entry{
		Content: memContent,
		Type:    memType,
		Metadata: map[string]interface{}{
			"saved_from": "cli",
		},
	}

	if err := store.Save(context.Background(), &entry); err != nil {
		return fmt.Errorf("save memory: %w", err)
	}

	fmt.Println("Saved to memory.")
	return nil
}

func runMemSearch() error {
	vs := newVectorStore()
	emb := embedding.NewLiteLLMEmbedder()
	store := memory.NewMemoryStore(vs, emb)

	entries, err := store.Search(context.Background(), memSearchQ, memLimit)
	if err != nil {
		return fmt.Errorf("search memory: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("No results.")
		return nil
	}

	for i, e := range entries {
		fmt.Printf("%d. [%s] %s\n", i+1, e.Type, e.Content)
	}
	return nil
}

func runMemClear() error {
	vs := newVectorStore()
	emb := embedding.NewLiteLLMEmbedder()
	store := memory.NewMemoryStore(vs, emb)

	if err := store.Clear(context.Background()); err != nil {
		return fmt.Errorf("clear memory: %w", err)
	}

	fmt.Println("Memory cleared.")
	return nil
}
