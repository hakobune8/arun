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
		registry.Register(adapter)
	}

	return nil
}
