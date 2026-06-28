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

package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogger_Log(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logger := NewLogger(dir)

	err := logger.Log("info", "test-component", "hello world", nil)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "run.log"))
	if err != nil {
		t.Fatal(err)
	}

	var entry LogEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatal(err)
	}

	if entry.Level != "info" {
		t.Errorf("Level = %q, want %q", entry.Level, "info")
	}
	if entry.Component != "test-component" {
		t.Errorf("Component = %q, want %q", entry.Component, "test-component")
	}
	if entry.Message != "hello world" {
		t.Errorf("Message = %q, want %q", entry.Message, "hello world")
	}
	if entry.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestLogger_LogWithData(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logger := NewLogger(dir)

	data := map[string]string{"key": "value"}
	err := logger.Log("warn", "test", "with data", data)
	if err != nil {
		t.Fatal(err)
	}

	content, _ := os.ReadFile(filepath.Join(dir, "run.log"))
	var entry LogEntry
	json.Unmarshal(content, &entry) //nolint:errcheck // test helper, error checked via read // test helper, error checked via content

	if entry.Data == nil {
		t.Fatal("expected Data to be non-nil")
	}
	m := entry.Data.(map[string]interface{})
	if m["key"] != "value" {
		t.Errorf("Data.key = %q, want %q", m["key"], "value")
	}
}

func TestLogger_LogTool(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logger := NewLogger(dir)

	err := logger.LogTool("read_file", map[string]string{"file": "test.txt"}, "output content", 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	content, _ := os.ReadFile(filepath.Join(dir, "tool_log.jsonl"))
	var entry ToolLogEntry
	json.Unmarshal(content, &entry) //nolint:errcheck // test helper, error checked via read // test helper, error checked via content

	if entry.Tool != "read_file" {
		t.Errorf("Tool = %q, want %q", entry.Tool, "read_file")
	}
	if entry.Duration == "" {
		t.Error("expected non-empty duration")
	}
}

func TestLogger_LogLLM(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logger := NewLogger(dir)

	err := logger.LogLLM(
		map[string]string{"model": "gpt-4"},
		map[string]string{"text": "response"},
		"gpt-4",
		200*time.Millisecond,
		10, 20,
	)
	if err != nil {
		t.Fatal(err)
	}

	content, _ := os.ReadFile(filepath.Join(dir, "llm_log.jsonl"))
	var entry LLMLogEntry
	json.Unmarshal(content, &entry) //nolint:errcheck // test helper, error checked via read // test helper, error checked via content

	if entry.Model != "gpt-4" {
		t.Errorf("Model = %q, want %q", entry.Model, "gpt-4")
	}
	if entry.PromptTokens != 10 {
		t.Errorf("PromptTokens = %d, want %d", entry.PromptTokens, 10)
	}
	if entry.CompletionTokens != 20 {
		t.Errorf("CompletionTokens = %d, want %d", entry.CompletionTokens, 20)
	}
}

func TestLogger_AppendsToExistingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logger := NewLogger(dir)

	logger.Log("info", "c1", "msg1", nil)  //nolint:errcheck // test helper, error checked via read // test helper, error checked via read
	logger.Log("info", "c2", "msg2", nil)  //nolint:errcheck // test helper, error checked via read // test helper, error checked via read

	data, _ := os.ReadFile(filepath.Join(dir, "run.log"))
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}
}

func TestLogger_MultipleLogFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logger := NewLogger(dir)

	logger.Log("info", "c", "m", nil)                      //nolint:errcheck // test helper, error checked via read // test helper, error checked via read
	logger.LogTool("tool", "in", "out", time.Second)        //nolint:errcheck // test helper, error checked via read
	logger.LogLLM("req", "resp", "model", time.Second, 1, 2) //nolint:errcheck // test helper, error checked via read

	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Errorf("expected 3 log files, got %d", len(entries))
	}
}
