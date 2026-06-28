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

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// ReadFileTool reads file contents from within the configured workspace.
type ReadFileTool struct {
	Workspace string
}

// NewReadFileTool creates a ReadFileTool rooted at workspace.
func NewReadFileTool(workspace string) *ReadFileTool {
	return &ReadFileTool{Workspace: workspace}
}

func (t *ReadFileTool) Name() string { return "read_file" }

func (t *ReadFileTool) Run(ctx context.Context, input ToolInput) ToolOutput {
	filePath, _ := input["file"].(string)
	if filePath == "" {
		return ToolOutput{Success: false, Error: "file is required"}
	}

	fullPath := filepath.Join(t.Workspace, filePath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return ToolOutput{Success: false, Error: fmt.Sprintf("read file: %v", err)}
	}

	return ToolOutput{Success: true, Data: string(data)}
}

// WriteFileTool writes file contents within the configured workspace.
type WriteFileTool struct {
	Workspace string
}

// NewWriteFileTool creates a WriteFileTool rooted at workspace.
func NewWriteFileTool(workspace string) *WriteFileTool {
	return &WriteFileTool{Workspace: workspace}
}

func (t *WriteFileTool) Name() string { return "write_file" }

func (t *WriteFileTool) Run(ctx context.Context, input ToolInput) ToolOutput {
	filePath, _ := input["file"].(string)
	if filePath == "" {
		return ToolOutput{Success: false, Error: "file is required"}
	}
	content, _ := input["content"].(string)
	if content == "" {
		return ToolOutput{Success: false, Error: "content is required"}
	}

	fullPath := filepath.Join(t.Workspace, filePath)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ToolOutput{Success: false, Error: fmt.Sprintf("create dir: %v", err)}
	}

	if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
		return ToolOutput{Success: false, Error: fmt.Sprintf("write file: %v", err)}
	}

	return ToolOutput{Success: true}
}
