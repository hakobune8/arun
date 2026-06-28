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
	"github.com/kazyamaz200/agentos/internal/guideline"
	"github.com/spf13/cobra"
)

var glCmd = &cobra.Command{
	Use:   "guideline",
	Short: "Coding guideline operations",
}

var glLoadCmd = &cobra.Command{
	Use:   "load",
	Short: "Load guidelines from a directory",
	Run: func(cmd *cobra.Command, args []string) {
		if err := runGlLoad(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var glSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search coding guidelines",
	Run: func(cmd *cobra.Command, args []string) {
		if err := runGlSearch(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var (
	glDir      string
	glQuery    string
	glLimit    int
)

func init() {
	glCmd.AddCommand(glLoadCmd)
	glCmd.AddCommand(glSearchCmd)
	rootCmd.AddCommand(glCmd)

	glLoadCmd.Flags().StringVarP(&glDir, "dir", "d", "guidelines", "Guidelines directory")
	glSearchCmd.Flags().StringVarP(&glQuery, "query", "q", "", "Search query")
	glSearchCmd.Flags().IntVarP(&glLimit, "limit", "l", 10, "Max results")
	_ = glSearchCmd.MarkFlagRequired("query") //nolint:errcheck // cobra returns error only for invalid flag name
}

func runGlLoad() error {
	vs := newVectorStore()
	emb := embedding.NewLiteLLMEmbedder()
	store := guideline.NewStore(vs, emb)

	if err := store.LoadDirectory(glDir); err != nil {
		return fmt.Errorf("load guidelines: %w", err)
	}

	fmt.Printf("Guidelines loaded from %s\n", glDir)
	return nil
}

func runGlSearch() error {
	vs := newVectorStore()
	emb := embedding.NewLiteLLMEmbedder()
	store := guideline.NewStore(vs, emb)

	gls, err := store.Search(context.Background(), glQuery, glLimit)
	if err != nil {
		return fmt.Errorf("search guidelines: %w", err)
	}

	if len(gls) == 0 {
		fmt.Println("No guidelines found.")
		return nil
	}

	for i, g := range gls {
		fmt.Printf("%d. %s\n", i+1, g.Title)
		fmt.Printf("   %s\n", g.Rule)
		if len(g.Tags) > 0 {
			fmt.Printf("   Tags: %v\n", g.Tags)
		}
		fmt.Println()
	}
	return nil
}
