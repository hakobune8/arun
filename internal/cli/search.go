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
	searchCmd.MarkFlagRequired("query")
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
