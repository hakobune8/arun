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

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewServer_ReturnsServer(t *testing.T) {
	t.Parallel()

	s := NewServer(0)
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
	if s.server == nil {
		t.Error("http.Server is nil")
	}
}

func TestNewServer_SetsPort(t *testing.T) {
	t.Parallel()

	s := NewServer(8080)
	if s.port != 8080 {
		t.Errorf("port = %d, want 8080", s.port)
	}
}

func TestServer_ServerAddr(t *testing.T) {
	t.Parallel()

	s := NewServer(9999)
	if s.server == nil {
		t.Fatal("http.Server is nil")
	}
	if s.server.Addr != ":9999" {
		t.Errorf("Addr = %q, want %q", s.server.Addr, ":9999")
	}
}

func TestServer_Shutdown_NotStarted(t *testing.T) {
	t.Parallel()

	s := NewServer(0)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := s.Shutdown(ctx)
	if err != nil && err != http.ErrServerClosed {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestServer_HealthEndpoint(t *testing.T) {
	t.Parallel()

	s := NewServer(0)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/health", http.NoBody)
	s.server.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("health status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestServer_SearchEndpoint_NoQuery(t *testing.T) {
	t.Parallel()

	s := NewServer(0)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/search", http.NoBody)
	s.server.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("search status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
