package mcp

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError  `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// InitializeParams contains the parameters for an MCP initialize request.
type InitializeParams struct {
	ClientInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

// InitializeResult contains the result of an MCP initialize request.
type InitializeResult struct {
	ServerInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
	Capabilities struct {
		Tools      *ToolsCapability `json:"tools,omitempty"`
		Resources  *interface{}     `json:"resources,omitempty"`
	} `json:"capabilities"`
}

// ToolsCapability describes the tools capability of an MCP server.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged"`
}

// ToolDefinition describes a tool exposed by an MCP server.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// ListToolsResult contains the result of a tools/list request.
type ListToolsResult struct {
	Tools []ToolDefinition `json:"tools"`
}

// CallToolParams contains the parameters for a tools/call request.
type CallToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// CallToolResult contains the result of a tool call.
type CallToolResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError"`
}

// ToolContent represents a piece of content returned by a tool.
type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
