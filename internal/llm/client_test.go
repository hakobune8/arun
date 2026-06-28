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

package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestMockLLMClient_Chat(t *testing.T) {
	t.Parallel()

	resps := []ChatResponse{
		{Choices: []Choice{{Message: Message{Role: RoleAssistant, Content: "hello"}}}},
		{Choices: []Choice{{Message: Message{Role: RoleAssistant, Content: "world"}}}},
	}
	m := NewMockLLMClient(resps)

	got1, err := m.Chat(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got1.Choices[0].Message.Content != "hello" {
		t.Errorf("got %q, want %q", got1.Choices[0].Message.Content, "hello")
	}

	got2, err := m.Chat(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got2.Choices[0].Message.Content != "world" {
		t.Errorf("got %q, want %q", got2.Choices[0].Message.Content, "world")
	}

	_, err = m.Chat(context.Background(), ChatRequest{})
	if err == nil {
		t.Fatal("expected error when no more responses")
	}
}

func TestMockLLMClient_ModelName(t *testing.T) {
	t.Parallel()

	m := NewMockLLMClient(nil)
	if got := m.ModelName(); got != "mock-model" {
		t.Errorf("ModelName() = %q, want %q", got, "mock-model")
	}
}

func TestLiteLLMClient_Config(t *testing.T) {
	t.Parallel()

	cfg := Config{BaseURL: "http://test:8080", APIKey: "key123", ModelCoder: "test-model", Timeout: 5 * time.Minute}
	c := NewLiteLLMClient(cfg)

	if got := c.Config(); got != cfg {
		t.Errorf("Config() = %+v, want %+v", got, cfg)
	}
	if got := c.ModelName(); got != "test-model" {
		t.Errorf("ModelName() = %q, want %q", got, "test-model")
	}
}

func TestLiteLLMClient_DefaultTimeout(t *testing.T) {
	t.Parallel()

	c := NewLiteLLMClient(Config{})
	if c.http.Timeout != 5*60*1e9 {
		t.Errorf("default timeout = %v, want 5m", c.http.Timeout)
	}
}

func TestLiteLLMClient_HTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`)) //nolint:errcheck // test mock handler
	}))
	defer srv.Close()

	c := NewLiteLLMClient(Config{BaseURL: srv.URL})
	_, err := c.Chat(context.Background(), ChatRequest{})
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
}

func TestLiteLLMClient_NoChoices(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[]}`)) //nolint:errcheck // test mock handler
	}))
	defer srv.Close()

	c := NewLiteLLMClient(Config{BaseURL: srv.URL})
	_, err := c.Chat(context.Background(), ChatRequest{})
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestLiteLLMClient_InvalidJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`)) //nolint:errcheck // test mock handler
	}))
	defer srv.Close()

	c := NewLiteLLMClient(Config{BaseURL: srv.URL})
	_, err := c.Chat(context.Background(), ChatRequest{})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLiteLLMClient_AuthHeader(t *testing.T) {
	t.Parallel()

	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ChatResponse{ //nolint:errcheck // test mock handler
			Choices: []Choice{{Message: Message{Role: RoleAssistant, Content: "ok"}}},
		})
	}))
	defer srv.Close()

	c := NewLiteLLMClient(Config{BaseURL: srv.URL, APIKey: "test-key"})
	_, err := c.Chat(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if authHeader != "Bearer test-key" {
		t.Errorf("Authorization header = %q, want %q", authHeader, "Bearer test-key")
	}
}

func TestDefaultConfig_Defaults(t *testing.T) {
	os.Unsetenv("LITELLM_BASE_URL")
	os.Unsetenv("LITELLM_API_KEY")
	os.Unsetenv("AGENTOS_MODEL_CODER")
	defer os.Setenv("LITELLM_BASE_URL", "")
	defer os.Setenv("LITELLM_API_KEY", "")
	defer os.Setenv("AGENTOS_MODEL_CODER", "")

	cfg := DefaultConfig()
	if cfg.BaseURL != "http://localhost:4000" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "http://localhost:4000")
	}
	if cfg.APIKey != "" {
		t.Errorf("APIKey = %q, want empty", cfg.APIKey)
	}
	if cfg.ModelCoder != "coder" {
		t.Errorf("ModelCoder = %q, want %q", cfg.ModelCoder, "coder")
	}
}
