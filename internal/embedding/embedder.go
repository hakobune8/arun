// Package embedding provides interfaces and implementations for text embeddings.
package embedding

import "context"

// Embedder defines the interface for generating text embeddings.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
	Model() string
}
