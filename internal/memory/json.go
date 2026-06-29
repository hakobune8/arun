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

package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// JSONStore implements Store using a local JSONL file with simple
// text-based search. No external dependencies required.
type JSONStore struct {
	mu      sync.RWMutex
	path    string
	entries []Entry
}

// NewJSONStore creates a JSONStore that persists entries to path.
func NewJSONStore(path string) (*JSONStore, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create store dir: %w", err)
	}

	s := &JSONStore{path: path}
	if data, err := os.ReadFile(path); err == nil {
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			var entry Entry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				return nil, fmt.Errorf("parse entry: %w", err)
			}
			s.entries = append(s.entries, entry)
		}
	}
	return s, nil
}

// Type returns "json" as the backend identifier.
func (s *JSONStore) Type() string { return "json" }

// Save stores a memory entry as a JSONL line.
func (s *JSONStore) Save(ctx context.Context, entry *Entry) error {
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("mem-%d", time.Now().UnixNano())
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries = append(s.entries, *entry)
	return s.flush()
}

// Search finds entries whose content contains the query (case-insensitive).
func (s *JSONStore) Search(ctx context.Context, query string, limit int) ([]Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	q := strings.ToLower(query)
	var results []Entry
	for _, entry := range s.entries {
		if strings.Contains(strings.ToLower(entry.Content), q) {
			results = append(results, entry)
		}
		if limit > 0 && len(results) >= limit {
			break
		}
	}

	// Sort by timestamp descending (most recent first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.After(results[j].Timestamp)
	})

	return results, nil
}

// Clear removes all entries.
func (s *JSONStore) Clear(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries = nil
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear store: %w", err)
	}
	return nil
}

func (s *JSONStore) flush() error {
	data := make([]byte, 0, 4096)
	for _, entry := range s.entries {
		line, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("marshal entry: %w", err)
		}
		data = append(data, line...)
		data = append(data, '\n')
	}
	return os.WriteFile(s.path, data, 0o600)
}
