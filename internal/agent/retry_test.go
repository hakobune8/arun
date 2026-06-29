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

package agent

import "testing"

func TestRetryConfig_Defaults(t *testing.T) {
	t.Parallel()
	var cfg RetryConfig
	if cfg.MaxRetries != 0 {
		t.Errorf("expected MaxRetries=0, got %d", cfg.MaxRetries)
	}
}

func TestRetryHandler_New(t *testing.T) {
	t.Parallel()
	cfg := RetryConfig{MaxRetries: 3}
	h := NewRetryHandler(cfg)
	if h == nil {
		t.Fatal("expected non-nil handler")
		return
	}
	if h.config.MaxRetries != 3 {
		t.Errorf("expected MaxRetries=3, got %d", h.config.MaxRetries)
	}
}

func TestRetryHandler_ShouldRetry_MaxRetriesExhausted(t *testing.T) {
	t.Parallel()
	h := NewRetryHandler(RetryConfig{MaxRetries: 3})
	got := h.ShouldRetry(3, true, true)
	if got {
		t.Error("expected false when attempt equals MaxRetries")
	}
}

func TestRetryHandler_ShouldRetry_TestFailed(t *testing.T) {
	t.Parallel()
	h := NewRetryHandler(RetryConfig{MaxRetries: 3})
	got := h.ShouldRetry(1, true, false)
	if !got {
		t.Error("expected true when test failed")
	}
}

func TestRetryHandler_ShouldRetry_LintFailed(t *testing.T) {
	t.Parallel()
	h := NewRetryHandler(RetryConfig{MaxRetries: 3})
	got := h.ShouldRetry(1, false, true)
	if !got {
		t.Error("expected true when lint failed")
	}
}

func TestRetryHandler_ShouldRetry_NoFailure(t *testing.T) {
	t.Parallel()
	h := NewRetryHandler(RetryConfig{MaxRetries: 3})
	got := h.ShouldRetry(1, false, false)
	if got {
		t.Error("expected false when nothing failed")
	}
}

func TestRetryHandler_ShouldRetry_ZeroMax(t *testing.T) {
	t.Parallel()
	h := NewRetryHandler(RetryConfig{MaxRetries: 0})
	got := h.ShouldRetry(0, true, true)
	if got {
		t.Error("expected false when MaxRetries is 0")
	}
}
