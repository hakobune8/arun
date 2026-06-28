package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SearchTool performs full-text search over Go source files within a
// workspace directory tree.
type SearchTool struct {
	Workspace string
}

// NewSearchTool creates a SearchTool rooted at workspace.
func NewSearchTool(workspace string) *SearchTool {
	return &SearchTool{Workspace: workspace}
}

func (t *SearchTool) Name() string { return "search" }

func (t *SearchTool) Run(ctx context.Context, input ToolInput) ToolOutput {
	pattern, _ := input["pattern"].(string)
	if pattern == "" {
		return ToolOutput{Success: false, Error: "pattern is required"}
	}
	searchPath, _ := input["path"].(string)
	if searchPath == "" {
		searchPath = "./"
	}

	fullPath := filepath.Join(t.Workspace, searchPath)
	var results []string

	err := filepath.Walk(fullPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
				return filepath.SkipDir
			}
			if info.Name() == "vendor" || info.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if strings.Contains(line, pattern) {
				relPath, _ := filepath.Rel(t.Workspace, path)
				results = append(results, fmt.Sprintf("%s:%d: %s", relPath, i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})

	if err != nil {
		return ToolOutput{Success: false, Error: fmt.Sprintf("search error: %v", err)}
	}

	return ToolOutput{Success: true, Data: results}
}
