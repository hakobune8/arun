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

// Package agent provides core agent interfaces and base implementations for coding agents.
package agent

// RetryConfig holds configuration for the retry behavior on failed tasks.
type RetryConfig struct {
	MaxRetries int
}

// RetryHandler determines whether to retry a task based on its configuration and attempt status.
type RetryHandler struct {
	config RetryConfig
}

// NewRetryHandler creates a new RetryHandler with the given configuration.
func NewRetryHandler(config RetryConfig) *RetryHandler {
	return &RetryHandler{config: config}
}

// ShouldRetry returns true if the task should be retried based on the current attempt count and failure status.
func (h *RetryHandler) ShouldRetry(attempt int, testFailed, lintFailed bool) bool {
	if attempt >= h.config.MaxRetries {
		return false
	}
	return testFailed || lintFailed
}
