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
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LogEntry is a generic log message with level, component, and optional
// structured data.
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Component string    `json:"component"`
	Message   string    `json:"message"`
	Data      any       `json:"data,omitempty"`
}

// ToolLogEntry records a tool invocation, its input, output, and duration.
type ToolLogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Tool      string    `json:"tool"`
	Input     any       `json:"input"`
	Output    any       `json:"output"`
	Duration  string    `json:"duration"`
}

// LLMLogEntry records an LLM API request/response pair including token
// counts and duration.
type LLMLogEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	Request    any       `json:"request"`
	Response   any       `json:"response"`
	Model      string    `json:"model"`
	Duration   string    `json:"duration"`
	PromptTokens int     `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
}

// Logger writes structured JSON log entries (generic logs, tool calls, LLM
// calls) to files in a run directory.
type Logger struct {
	runDir string
}

// NewLogger returns a Logger that writes to files in runDir.
func NewLogger(runDir string) *Logger {
	return &Logger{runDir: runDir}
}

// Log appends a LogEntry to run.log.
func (l *Logger) Log(level, component, message string, data any) error {
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Component: component,
		Message:   message,
		Data:      data,
	}
	return l.appendJSON("run.log", entry)
}

// LogTool appends a ToolLogEntry to tool_log.jsonl.
func (l *Logger) LogTool(tool string, input, output any, duration time.Duration) error {
	entry := ToolLogEntry{
		Timestamp: time.Now(),
		Tool:      tool,
		Input:     input,
		Output:    output,
		Duration:  duration.String(),
	}
	return l.appendJSON("tool_log.jsonl", entry)
}

// LogLLM appends an LLMLogEntry to llm_log.jsonl.
func (l *Logger) LogLLM(req, resp any, model string, duration time.Duration, promptTokens, completionTokens int) error {
	entry := LLMLogEntry{
		Timestamp:        time.Now(),
		Request:          req,
		Response:         resp,
		Model:            model,
		Duration:         duration.String(),
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
	}
	return l.appendJSON("llm_log.jsonl", entry)
}

func (l *Logger) appendJSON(filename string, entry any) error {
	path := filepath.Join(l.runDir, filename)
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal log entry: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write log: %w", err)
	}
	if _, err := f.WriteString("\n"); err != nil {
		return fmt.Errorf("write newline: %w", err)
	}
	return nil
}
