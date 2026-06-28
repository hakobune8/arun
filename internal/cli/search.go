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
	"path/filepath"

	"github.com/kazyamaz200/agentos/internal/embedding"
	"github.com/kazyamaz200/agentos/internal/search"
	"github.com/kazyamaz200/agentos/internal/vector"
	"github.com/spf13/cobra"
)

var (
	searchQuery  string
	searchType   string
	searchLimit  int
)

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search across memory, guidelines, and past PRs",
	Run: func(cmd *cobra.Command, args []string) {
		if err := runSearch(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
	searchCmd.Flags().StringVarP(&searchQuery, "query", "q", "", "Search query")
	searchCmd.Flags().StringVarP(&searchType, "type", "t", "all", "Search type (memory/guideline/pr/all)")
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "l", 10, "Max results")
	_ = searchCmd.MarkFlagRequired("query") //nolint:errcheck // cobra returns error only for invalid flag name
}

func newVectorStore() vector.VectorStore {
	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL != "" {
		return vector.NewQdrantClient()
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return vector.NewLocalStore(filepath.Join(home, ".agentos", "vectors"))
}

func runSearch() error {
	vs := newVectorStore()
	emb := embedding.NewLiteLLMEmbedder()
	svc := search.NewService(vs, emb)

	results, err := svc.Search(context.Background(), searchQuery, search.Type(searchType), searchLimit)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		return nil
	}

	for i, r := range results {
		fmt.Printf("%d. [%s] (score: %.4f)\n", i+1, r.Source, r.Score)
		fmt.Printf("   %s\n", truncate(r.Content, 120))
		if r.Metadata != nil {
			if title, ok := r.Metadata["title"]; ok {
				fmt.Printf("   Title: %s\n", title)
			}
		}
		fmt.Println()
	}

	return nil
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}
