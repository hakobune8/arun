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

package vector

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestNewLocalStore_CreatesDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewLocalStore(dir)

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("NewLocalStore did not create the directory")
	}
	if store.Name() != "local" {
		t.Errorf("Name() = %q, want %q", store.Name(), "local")
	}
}

func TestLocalStore_Upsert_AddsAndUpdates(t *testing.T) {
	t.Parallel()

	store := NewLocalStore(t.TempDir())
	ctx := context.Background()

	err := store.Upsert(ctx, "test_coll", []Point{
		{ID: "p1", Vector: []float32{1, 0, 0}, Payload: map[string]interface{}{"val": "a"}},
	})
	if err != nil {
		t.Fatalf("Upsert add: %v", err)
	}

	err = store.Upsert(ctx, "test_coll", []Point{
		{ID: "p1", Vector: []float32{1, 0, 0}, Payload: map[string]interface{}{"val": "b"}},
		{ID: "p2", Vector: []float32{0, 1, 0}, Payload: map[string]interface{}{"val": "c"}},
	})
	if err != nil {
		t.Fatalf("Upsert update: %v", err)
	}

	results, err := store.Search(ctx, "test_coll", []float32{1, 0, 0}, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Payload["val"] != "b" {
		t.Errorf("updated point payload = %v, want %v", results[0].Payload["val"], "b")
	}
}

func TestLocalStore_Search_ReturnsSortedByCosineSimilarity(t *testing.T) {
	t.Parallel()

	store := NewLocalStore(t.TempDir())
	ctx := context.Background()

	store.Upsert(ctx, "sim_coll", []Point{ //nolint:errcheck // test helper, error checked via search
		{ID: "close", Vector: []float32{0.9, 0.1, 0}},
		{ID: "far", Vector: []float32{0.1, 0.9, 0}},
	})

	results, err := store.Search(ctx, "sim_coll", []float32{1, 0, 0}, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].ID != "close" {
		t.Errorf("top result = %q, want %q", results[0].ID, "close")
	}
	if results[0].Score < results[1].Score {
		t.Error("results not sorted descending by score")
	}
}

func TestLocalStore_Search_EmptyCollectionReturnsNil(t *testing.T) {
	t.Parallel()

	store := NewLocalStore(t.TempDir())
	ctx := context.Background()

	results, err := store.Search(ctx, "nonexistent", []float32{1, 0, 0}, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if results != nil {
		t.Errorf("Search returned %v, want nil", results)
	}
}

func TestLocalStore_DeleteCollection_RemovesFile(t *testing.T) {
	t.Parallel()

	store := NewLocalStore(t.TempDir())
	ctx := context.Background()

	store.Upsert(ctx, "del_coll", []Point{ //nolint:errcheck // test helper, error checked via search
		{ID: "p1", Vector: []float32{1, 0, 0}},
	})

	path := filepath.Join(store.dir, "del_coll.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("collection file should exist before delete")
	}

	err := store.DeleteCollection(ctx, "del_coll")
	if err != nil {
		t.Fatalf("DeleteCollection: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("collection file should be removed after delete")
	}
}

func TestDeleteCollection_NonExistent_ReturnsNil(t *testing.T) {
	t.Parallel()

	store := NewLocalStore(t.TempDir())
	err := store.DeleteCollection(context.Background(), "no_such_file")
	if err != nil {
		t.Errorf("DeleteCollection on nonexistent: %v", err)
	}
}

func TestCosineSimilarity_EqualVectors(t *testing.T) {
	t.Parallel()

	a := []float32{1, 2, 3}
	b := []float32{1, 2, 3}
	s := cosineSimilarity(a, b)
	if s != 1.0 {
		t.Errorf("cosineSimilarity(equal) = %f, want 1.0", s)
	}
}

func TestCosineSimilarity_OppositeVectors(t *testing.T) {
	t.Parallel()

	a := []float32{1, 0, 0}
	b := []float32{-1, 0, 0}
	s := cosineSimilarity(a, b)
	if math.Abs(s-(-1.0)) > 1e-6 {
		t.Errorf("cosineSimilarity(opposite) = %f, want -1.0", s)
	}
}

func TestCosineSimilarity_EmptyVectors(t *testing.T) {
	t.Parallel()

	if s := cosineSimilarity(nil, []float32{1, 2}); s != 0 {
		t.Errorf("cosineSimilarity(nil, non-nil) = %f, want 0", s)
	}
	if s := cosineSimilarity([]float32{1, 2}, nil); s != 0 {
		t.Errorf("cosineSimilarity(non-nil, nil) = %f, want 0", s)
	}
}

func TestCosineSimilarity_DifferentLengthVectors(t *testing.T) {
	t.Parallel()

	a := []float32{1, 2, 3}
	b := []float32{1, 2}
	if s := cosineSimilarity(a, b); s != 0 {
		t.Errorf("cosineSimilarity(diff length) = %f, want 0", s)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	t.Parallel()

	a := []float32{0, 0, 0}
	b := []float32{1, 2, 3}
	if s := cosineSimilarity(a, b); s != 0 {
		t.Errorf("cosineSimilarity(zero, non-zero) = %f, want 0", s)
	}
}
