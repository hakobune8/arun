package llm

import (
	"os"
)

func DefaultConfig() Config {
	baseURL := os.Getenv("LITELLM_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:4000"
	}
	apiKey := os.Getenv("LITELLM_API_KEY")
	modelCoder := os.Getenv("AGENTOS_MODEL_CODER")
	if modelCoder == "" {
		modelCoder = "coder"
	}
	return Config{
		BaseURL:    baseURL,
		APIKey:     apiKey,
		ModelCoder: modelCoder,
	}
}
