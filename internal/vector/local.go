package vector

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type LocalStore struct {
	dir string
	mu  sync.RWMutex
}

func NewLocalStore(dir string) *LocalStore {
	os.MkdirAll(dir, 0755)
	return &LocalStore{dir: dir}
}

func (s *LocalStore) Name() string { return "local" }

func (s *LocalStore) collectionPath(collection string) string {
	return filepath.Join(s.dir, collection+".json")
}

func (s *LocalStore) Upsert(ctx context.Context, collection string, points []Point) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing := s.loadCollection(collection)
	idx := make(map[string]int)
	for i, p := range existing {
		idx[p.ID] = i
	}

	for _, p := range points {
		if i, ok := idx[p.ID]; ok {
			existing[i] = p
		} else {
			idx[p.ID] = len(existing)
			existing = append(existing, p)
		}
	}

	return s.saveCollection(collection, existing)
}

func (s *LocalStore) Search(ctx context.Context, collection string, query []float32, limit int) ([]Point, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	points := s.loadCollection(collection)
	if len(points) == 0 {
		return nil, nil
	}

	type scored struct {
		point Point
		score float64
	}

	var results []scored
	for _, p := range points {
		score := cosineSimilarity(query, p.Vector)
		results = append(results, scored{p, score})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if limit > 0 && limit < len(results) {
		results = results[:limit]
	}

	out := make([]Point, len(results))
	for i, r := range results {
		out[i] = r.point
		out[i].Score = r.score
	}
	return out, nil
}

func (s *LocalStore) DeleteCollection(ctx context.Context, collection string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.collectionPath(collection)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete collection: %w", err)
	}
	return nil
}

func (s *LocalStore) loadCollection(name string) []Point {
	path := s.collectionPath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var points []Point
	if err := json.Unmarshal(data, &points); err != nil {
		return nil
	}
	return points
}

func (s *LocalStore) saveCollection(name string, points []Point) error {
	path := s.collectionPath(name)
	data, err := json.MarshalIndent(points, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		da := float64(a[i])
		db := float64(b[i])
		dot += da * db
		na += da * da
		nb += db * db
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
