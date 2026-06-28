// Package llm provides LLM client interfaces and implementations for interacting
// with language models via LiteLLM.
package llm

import "context"

// Role represents a message role in a chat conversation (system, user, or assistant).
type Role string

const (
	// RoleSystem indicates a system-level message providing instructions to the model.
	RoleSystem Role = "system"
	// RoleUser indicates a user message containing the input or query.
	RoleUser Role = "user"
	// RoleAssistant indicates an assistant (model) response message.
	RoleAssistant Role = "assistant"
)

// Message represents a single message in a chat conversation with role and content.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// ChatRequest represents a request to the LLM chat completion API.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

// Choice represents a single response choice from the LLM, including the message content.
type Choice struct {
	Index   int     `json:"index"`
	Message Message `json:"message"`
}

// Usage contains token usage statistics for an LLM API call.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatResponse represents a response from the LLM chat completion API.
type ChatResponse struct {
	Choices []Choice `json:"choices"`
	Usage   *Usage   `json:"usage,omitempty"`
}

// LLMClient defines the interface for interacting with a language model.
type LLMClient interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	ModelName() string
}
