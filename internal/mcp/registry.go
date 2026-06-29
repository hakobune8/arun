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

package mcp

import (
	"context"
	"fmt"

	"github.com/kazyamaz200/agentos/internal/tools"
)

// ToolAdapter adapts an MCP tool to the local tools.Registry interface.
type ToolAdapter struct {
	client *Client
	def    ToolDefinition
}

// Name returns the prefixed name of the MCP tool.
func (a *ToolAdapter) Name() string {
	return "mcp_" + a.def.Name
}

// Description returns the tool's description from the MCP definition.
func (a *ToolAdapter) Description() string {
	return a.def.Description
}

func (a *ToolAdapter) Run(ctx context.Context, input tools.ToolInput) tools.ToolOutput {
	args := make(map[string]interface{})
	for k, v := range input {
		args[k] = v
	}

	result, err := a.client.CallTool(ctx, a.def.Name, args)
	if err != nil {
		return tools.ToolOutput{
			Success: false,
			Error:   fmt.Sprintf("MCP tool %s: %v", a.def.Name, err),
		}
	}

	text := ""
	for _, c := range result.Content {
		text += c.Text + "\n"
	}

	return tools.ToolOutput{
		Success: !result.IsError,
		Data:    text,
	}
}

// RegisterMCPServer connects to an MCP server and registers all its tools in the given registry.
func RegisterMCPServer(registry *tools.Registry, client *Client) error {
	ctx := context.Background()
	defs, err := client.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("list MCP tools: %w", err)
	}

	for _, def := range defs {
		adapter := &ToolAdapter{
			client: client,
			def:    def,
		}
		if err := registry.Register(adapter); err != nil {
			return fmt.Errorf("register MCP tool %q: %w", def.Name, err)
		}
	}

	return nil
}
