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

package event

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FileStore persists events to a JSONL file and implements Bus as a write-only layer.
// It is typically used as a subscription to the main bus for durable storage.
type FileStore struct {
	path string
	file *os.File
}

// NewFileStore creates a FileStore that appends events to path.
func NewFileStore(path string) (*FileStore, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create event store dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open event store: %w", err)
	}
	return &FileStore{path: path, file: f}, nil
}

// Handler returns an event Handler that writes events to the file.
func (s *FileStore) Handler() Handler {
	return func(ctx context.Context, e Event) error {
		data, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("marshal event: %w", err)
		}
		if _, err := s.file.Write(data); err != nil {
			return fmt.Errorf("write event: %w", err)
		}
		if _, err := s.file.WriteString("\n"); err != nil {
			return fmt.Errorf("write newline: %w", err)
		}
		return nil
	}
}

// Replay reads all events from the file and sends them to the handler.
func (s *FileStore) Replay(ctx context.Context, handler Handler) error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read event store: %w", err)
	}

	lines := splitLines(string(data))
	for _, line := range lines {
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return fmt.Errorf("unmarshal event: %w", err)
		}
		if err := handler(ctx, e); err != nil {
			return fmt.Errorf("replay event handler: %w", err)
		}
	}
	return nil
}

// Close closes the underlying file.
func (s *FileStore) Close() error {
	if s.file != nil {
		return s.file.Close()
	}
	return nil
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i > start {
				lines = append(lines, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
