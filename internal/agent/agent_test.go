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

import (
	"testing"

	"github.com/kazyamaz200/agentos/internal/llm"
)

func TestBaseAgent_New(t *testing.T) {
	t.Parallel()
	mock := llm.NewMockLLMClient(nil)
	a := NewBaseAgent("test-agent", mock)
	if a == nil {
		t.Fatal("expected non-nil agent")
	}
	if a.name != "test-agent" {
		t.Errorf("expected name 'test-agent', got %q", a.name)
	}
}

func TestBaseAgent_Name(t *testing.T) {
	t.Parallel()
	mock := llm.NewMockLLMClient(nil)
	a := NewBaseAgent("my-agent", mock)
	if got := a.Name(); got != "my-agent" {
		t.Errorf("expected 'my-agent', got %q", got)
	}
}

func TestBaseAgent_ImplementsRuntimeAgent(t *testing.T) {
	t.Parallel()
	mock := llm.NewMockLLMClient(nil)
	a := NewBaseAgent("test", mock)
	// Compile-time check: assert that *BaseAgent satisfies the interface.
	var _ interface {
		Name() string
	} = a
	_ = a.Name()
}
