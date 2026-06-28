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
	glSearchCmd.MarkFlagRequired("query")
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
