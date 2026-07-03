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

package mcp

import (
	"testing"
)

func TestNewClient_CreatesClient(t *testing.T) {
	t.Parallel()

	c := NewClient("echo", "hello")
	if c == nil {
		t.Fatal("NewClient returned nil")
	}

	if c.IsConnected() {
		t.Error("new client should not be connected")
	}
	if c.Info() != nil {
		t.Error("new client should have nil info")
	}
}

func TestNewClient_NoArgs(t *testing.T) {
	t.Parallel()

	c := NewClient("true")
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	if c.IsConnected() {
		t.Error("new client should not be connected")
	}
}

func TestNewClient_CommandNotStarted(t *testing.T) {
	t.Parallel()

	c := NewClient("nonexistent-command-12345")
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	// Should not panic - no process started
}
