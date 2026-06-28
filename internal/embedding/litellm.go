package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type LiteLLMEmbedder struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

func NewLiteLLMEmbedder() *LiteLLMEmbedder {
	baseURL := os.Getenv("LITELLM_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:4000"
	}
	apiKey := os.Getenv("LITELLM_API_KEY")
	if apiKey == "" {
		apiKey = "sk-local"
	}
	model := os.Getenv("AGENTOS_MODEL_EMBEDDING")
	if model == "" {
		model = "text-embedding-ada-002"
	}

	return &LiteLLMEmbedder{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Usage *struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

func (e *LiteLLMEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	req := embedRequest{
		Model: e.model,
		Input: texts,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("embedding API error (status %d): %s", resp.StatusCode, string(respData))
	}

	var embedResp embedResponse
	if err := json.Unmarshal(respData, &embedResp); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	if len(embedResp.Data) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	result := make([][]float32, len(embedResp.Data))
	for i, d := range embedResp.Data {
		result[i] = d.Embedding
	}

	return result, nil
}

func (e *LiteLLMEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := e.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

func (e *LiteLLMEmbedder) Model() string {
	return e.model
}
