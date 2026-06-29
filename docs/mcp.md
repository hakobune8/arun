# MCP Client

AgentOS supports the Model Context Protocol (MCP) for integrating with
external tools via JSON-RPC over stdio.

## Architecture

```
AgentOS Runtime → Tool Registry → MCP ToolAdapter → MCP Server (stdio)
```

## Registering an MCP Server

```go
client, _ := mcp.NewClient("npx", []string{"@modelcontextprotocol/server-filesystem", "/path"})
defer client.Close()

defs, _ := client.ListTools()
registry := tools.NewRegistry()
mcp.RegisterMCPServer(registry, client, defs)
```

## Tool Naming

MCP tools are registered with an `mcp_` prefix to avoid naming conflicts:

| MCP Tool Name | Registry Name |
|---------------|---------------|
| `read_file` | `mcp_read_file` |
| `write_file` | `mcp_write_file` |

## Protocol

MCP communication uses JSON-RPC 2.0 over the server's stdin/stdout.
Supported methods:
- `initialize` — handshake
- `tools/list` — list available tools
- `tools/call` — execute a tool with arguments
