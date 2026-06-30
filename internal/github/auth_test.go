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

package github

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAppInstallationTokenProvider_TokenCachesInstallationToken(t *testing.T) {
	t.Parallel()

	keyPEM := testRSAPrivateKeyPEM(t)
	var tokenRequests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/installations/99/access_tokens" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.Count(strings.TrimPrefix(auth, "Bearer "), ".") != 2 {
			t.Fatalf("expected JWT bearer auth, got %q", auth)
		}
		atomic.AddInt32(&tokenRequests, 1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck // test helper
			"token":      "installation-token",
			"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	}))
	defer ts.Close()

	provider, err := NewAppInstallationTokenProvider(&AppConfig{
		AppID:          "123",
		InstallationID: "99",
		PrivateKeyPEM:  keyPEM,
		BaseURL:        ts.URL,
		HTTPClient:     ts.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		token, err := provider.Token(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if token != "installation-token" {
			t.Fatalf("token = %q, want installation-token", token)
		}
	}
	if got := atomic.LoadInt32(&tokenRequests); got != 1 {
		t.Fatalf("token endpoint calls = %d, want 1", got)
	}
}

func TestClient_UsesGitHubAppInstallationToken(t *testing.T) {
	keyPEM := testRSAPrivateKeyPEM(t)
	var tokenRequests int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/app/installations/99/access_tokens":
			atomic.AddInt32(&tokenRequests, 1)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck // test helper
				"token":      "installation-token",
				"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			})
		case "/repos/owner/repo/issues":
			if got := r.Header.Get("Authorization"); got != "Bearer installation-token" {
				t.Fatalf("Authorization = %q, want bearer installation token", got)
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[]`)) //nolint:errcheck // test helper
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	t.Setenv("GITHUB_API_URL", ts.URL)
	t.Setenv("GITHUB_TOKEN", "personal-token")
	t.Setenv("GITHUB_APP_ID", "123")
	t.Setenv("GITHUB_APP_INSTALLATION_ID", "99")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", keyPEM)

	c := NewClient("owner", "repo")
	if c.Token != "installation-token" {
		t.Fatalf("client token = %q, want installation-token", c.Token)
	}
	if _, err := c.ListIssues("open"); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&tokenRequests); got != 1 {
		t.Fatalf("token endpoint calls = %d, want 1", got)
	}
}

func TestTokenFromEnv_ReturnsErrorForInvalidGitHubAppKey(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "personal-token")
	t.Setenv("GITHUB_APP_ID", "123")
	t.Setenv("GITHUB_APP_INSTALLATION_ID", "99")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", "not a pem key")

	_, err := TokenFromEnv(context.Background())
	if err == nil {
		t.Fatal("expected invalid GitHub App private key error")
	}
	if !strings.Contains(err.Error(), "private key") {
		t.Fatalf("expected private key error, got %v", err)
	}
}

func testRSAPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	return string(pem.EncodeToMemory(block))
}
