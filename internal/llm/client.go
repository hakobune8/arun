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

// Package llm provides LLM client interfaces and implementations for interacting
// with language models via LiteLLM.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Config holds the configuration for connecting to a LiteLLM proxy server.
type Config struct {
	BaseURL    string
	APIKey     string
	ModelCoder string
	Timeout    time.Duration
}

// LiteLLMClient is an HTTP client for the LiteLLM proxy API that implements LLMClient.
type LiteLLMClient struct {
	config Config
	http   *http.Client
}

// NewLiteLLMClient creates a new LiteLLMClient with the given config, defaulting to a 5-minute timeout.
func NewLiteLLMClient(config Config) *LiteLLMClient {
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Minute
	}
	return &LiteLLMClient{
		config: config,
		http: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// Chat sends a chat completion request to the LiteLLM proxy and returns the response.
func (c *LiteLLMClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LLM API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in LLM response")
	}

	return &chatResp, nil
}

// ModelName returns the configured model name for coding tasks.
func (c *LiteLLMClient) ModelName() string {
	return c.config.ModelCoder
}

// Config returns the current client configuration.
func (c *LiteLLMClient) Config() Config {
	return c.config
}

// MockLLMClient is a test double that returns pre-configured responses in sequence.
type MockLLMClient struct {
	Responses []ChatResponse
	Requests  []ChatRequest
	Index     int
}

// NewMockLLMClient creates a new MockLLMClient with the given slice of responses to return in order.
func NewMockLLMClient(responses []ChatResponse) *MockLLMClient {
	return &MockLLMClient{Responses: responses}
}

// Chat returns the next pre-configured mock response in the sequence.
func (m *MockLLMClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	m.Requests = append(m.Requests, req)
	if m.Index >= len(m.Responses) {
		return nil, fmt.Errorf("no more mock responses")
	}
	resp := m.Responses[m.Index]
	m.Index++
	return &resp, nil
}

// ModelName returns "mock-model" as the mock client identifier.
func (m *MockLLMClient) ModelName() string {
	return "mock-model"
}
