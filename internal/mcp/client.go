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

// Package mcp implements a Model Context Protocol client for interacting with MCP servers.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Client represents a connection to an MCP server process.
type Client struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	reader    *bufio.Scanner
	mu        sync.Mutex
	nextID    int
	connected bool
	info      *InitializeResult
}

// NewClient creates a new MCP client for the given command.
func NewClient(command string, args ...string) *Client {
	cmd := exec.Command(command, args...)
	return &Client{cmd: cmd}
}

// Connect starts the MCP server process and performs the initialization handshake.
func (c *Client) Connect(ctx context.Context) error {
	stdin, err := c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	c.cmd.Stderr = nil
	c.stdin = stdin
	c.stdout = stdout
	c.reader = bufio.NewScanner(stdout)
	c.reader.Buffer(make([]byte, 1024*1024), 1024*1024)

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("start MCP server: %w", err)
	}

	initReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: InitializeParams{
			ClientInfo: struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}{
				Name:    "agentos",
				Version: "0.4.0",
			},
		},
	}

	var result InitializeResult
	if err := c.call(ctx, initReq, &result); err != nil {
		c.Close()
		return fmt.Errorf("initialize: %w", err)
	}
	c.info = &result
	c.connected = true

	notif := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	_ = c.send(notif)

	return nil
}

// ListTools retrieves the list of available tools from the MCP server.
func (c *Client) ListTools(ctx context.Context) ([]ToolDefinition, error) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID,
		Method:  "tools/list",
	}
	c.nextID++

	var result ListToolsResult
	if err := c.call(ctx, req, &result); err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}
	return result.Tools, nil
}

// CallTool invokes a tool on the MCP server with the given arguments.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID,
		Method:  "tools/call",
		Params: CallToolParams{
			Name:      name,
			Arguments: args,
		},
	}
	c.nextID++

	var result CallToolResult
	if err := c.call(ctx, req, &result); err != nil {
		return nil, fmt.Errorf("call tool %s: %w", name, err)
	}
	return &result, nil
}

// Close terminates the MCP server process.
func (c *Client) Close() error {
	c.connected = false
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill() //nolint:errcheck // best-effort cleanup
		_ = c.cmd.Wait()         //nolint:errcheck // best-effort cleanup
	}
	return nil
}

// IsConnected returns whether the client is currently connected to the MCP server.
func (c *Client) IsConnected() bool {
	return c.connected
}

// Info returns the server info received during initialization.
func (c *Client) Info() *InitializeResult {
	return c.info
}

func (c *Client) call(ctx context.Context, req JSONRPCRequest, result interface{}) error {
	if err := c.send(req); err != nil {
		return err
	}
	return c.receive(ctx, req.ID, result)
}

func (c *Client) send(req JSONRPCRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if _, err := c.stdin.Write([]byte("\n")); err != nil {
		return fmt.Errorf("write newline: %w", err)
	}
	return nil
}

func (c *Client) receive(ctx context.Context, id int, result interface{}) error {
	done := make(chan error, 1)
	go func() {
		for c.reader.Scan() {
			line := strings.TrimSpace(c.reader.Text())
			if line == "" {
				continue
			}

			var resp JSONRPCResponse
			if err := json.Unmarshal([]byte(line), &resp); err != nil {
				continue
			}

			if resp.ID != id {
				continue
			}

			if resp.Error != nil {
				done <- fmt.Errorf("RPC error %d: %s", resp.Error.Code, resp.Error.Message)
				return
			}

			data, err := json.Marshal(resp.Result)
			if err != nil {
				done <- fmt.Errorf("marshal result: %w", err)
				return
			}

			if result != nil {
				if err := json.Unmarshal(data, result); err != nil {
					done <- fmt.Errorf("unmarshal result: %w", err)
					return
				}
			}

			done <- nil
			return
		}
		done <- fmt.Errorf("connection closed")
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout waiting for response")
	}
}
