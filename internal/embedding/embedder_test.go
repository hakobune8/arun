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

package embedding

import (
	"context"
	"testing"
)

// mockEmbedder implements the Embedder interface with fixed vectors.
type mockEmbedder struct {
	fixedVec []float32
	model    string
}

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = m.fixedVec
	}
	return result, nil
}

func (m *mockEmbedder) EmbedQuery(_ context.Context, _ string) ([]float32, error) {
	return m.fixedVec, nil
}

func (m *mockEmbedder) Model() string {
	return m.model
}

func TestMockEmbedder_Embed(t *testing.T) {
	t.Parallel()

	m := &mockEmbedder{
		fixedVec: []float32{0.1, 0.2, 0.3},
		model:    "test-model",
	}
	ctx := context.Background()

	vecs, err := m.Embed(ctx, []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("got %d vectors, want 2", len(vecs))
	}
	for i, v := range vecs {
		if len(v) != 3 {
			t.Errorf("vector %d length = %d, want 3", i, len(v))
		}
		if v[0] != 0.1 || v[1] != 0.2 || v[2] != 0.3 {
			t.Errorf("vector %d = %v, want [0.1 0.2 0.3]", i, v)
		}
	}
}

func TestMockEmbedder_EmbedQuery(t *testing.T) {
	t.Parallel()

	m := &mockEmbedder{
		fixedVec: []float32{0.4, 0.5, 0.6},
	}
	ctx := context.Background()

	v, err := m.EmbedQuery(ctx, "test query")
	if err != nil {
		t.Fatalf("EmbedQuery: %v", err)
	}
	if len(v) != 3 {
		t.Fatalf("vector length = %d, want 3", len(v))
	}
	if v[0] != 0.4 || v[1] != 0.5 || v[2] != 0.6 {
		t.Errorf("vector = %v, want [0.4 0.5 0.6]", v)
	}
}

func TestMockEmbedder_Model(t *testing.T) {
	t.Parallel()

	m := &mockEmbedder{model: "custom-model"}
	if m.Model() != "custom-model" {
		t.Errorf("Model() = %q, want %q", m.Model(), "custom-model")
	}
}
