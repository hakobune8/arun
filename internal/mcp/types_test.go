// Copyright 2026 ARUN Authors
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
	"encoding/json"
	"testing"
)

func TestInitializeResult_StructCreation(t *testing.T) {
	t.Parallel()

	result := InitializeResult{}
	result.ServerInfo.Name = "test-server"
	result.ServerInfo.Version = "1.0.0"
	result.Capabilities.Tools = &ToolsCapability{ListChanged: true}

	if result.ServerInfo.Name != "test-server" {
		t.Errorf("ServerInfo.Name = %q, want %q", result.ServerInfo.Name, "test-server")
	}
	if result.ServerInfo.Version != "1.0.0" {
		t.Errorf("ServerInfo.Version = %q, want %q", result.ServerInfo.Version, "1.0.0")
	}
	if result.Capabilities.Tools == nil || !result.Capabilities.Tools.ListChanged {
		t.Error("Tools.ListChanged should be true")
	}
}

func TestTool_StructCreation(t *testing.T) {
	t.Parallel()

	tool := ToolDefinition{
		Name:        "get_weather",
		Description: "Get weather for a location",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"location": map[string]interface{}{"type": "string"},
			},
		},
	}

	if tool.Name != "get_weather" {
		t.Errorf("Name = %q, want %q", tool.Name, "get_weather")
	}
	if tool.Description != "Get weather for a location" {
		t.Errorf("Description = %q, want %q", tool.Description, "Get weather for a location")
	}
}

func TestCallToolResult_StructCreation(t *testing.T) {
	t.Parallel()

	result := CallToolResult{
		Content: []ToolContent{
			{Type: "text", Text: "Hello, world!"},
		},
		IsError: false,
	}

	if result.IsError {
		t.Error("IsError should be false")
	}
	if len(result.Content) != 1 {
		t.Fatalf("got %d content items, want 1", len(result.Content))
	}
	if result.Content[0].Type != "text" {
		t.Errorf("Content[0].Type = %q, want %q", result.Content[0].Type, "text")
	}
	if result.Content[0].Text != "Hello, world!" {
		t.Errorf("Content[0].Text = %q, want %q", result.Content[0].Text, "Hello, world!")
	}
}

func TestJSONRPCRequest_Marshal(t *testing.T) {
	t.Parallel()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded JSONRPCRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q, want %q", decoded.JSONRPC, "2.0")
	}
	if decoded.ID != 1 {
		t.Errorf("ID = %d, want 1", decoded.ID)
	}
	if decoded.Method != "tools/list" {
		t.Errorf("Method = %q, want %q", decoded.Method, "tools/list")
	}
}

func TestJSONRPCResponse_Marshal(t *testing.T) {
	t.Parallel()

	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Result: map[string]interface{}{
			"status": "ok",
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded JSONRPCResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q, want %q", decoded.JSONRPC, "2.0")
	}
}

func TestJSONRPCResponse_WithError(t *testing.T) {
	t.Parallel()

	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Error: &RPCError{
			Code:    -32601,
			Message: "Method not found",
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded JSONRPCResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Error == nil {
		t.Fatal("Error should not be nil")
	}
	if decoded.Error.Code != -32601 {
		t.Errorf("Error.Code = %d, want %d", decoded.Error.Code, -32601)
	}
	if decoded.Error.Message != "Method not found" {
		t.Errorf("Error.Message = %q, want %q", decoded.Error.Message, "Method not found")
	}
}

func TestToolContent_Marshal(t *testing.T) {
	t.Parallel()

	content := ToolContent{Type: "text", Text: "response text"}
	data, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ToolContent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Type != "text" {
		t.Errorf("Type = %q, want %q", decoded.Type, "text")
	}
	if decoded.Text != "response text" {
		t.Errorf("Text = %q, want %q", decoded.Text, "response text")
	}
}
