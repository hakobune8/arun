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
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const installationTokenRefreshSkew = 5 * time.Minute

// TokenProvider returns a bearer token for GitHub API or Git HTTPS requests.
type TokenProvider interface {
	Token(ctx context.Context) (string, error)
}

type staticTokenProvider struct {
	token string
}

func (p staticTokenProvider) Token(context.Context) (string, error) {
	return p.token, nil
}

type errorTokenProvider struct {
	err error
}

func (p errorTokenProvider) Token(context.Context) (string, error) {
	return "", p.err
}

// AppConfig contains the settings required to authenticate as a GitHub App.
type AppConfig struct {
	AppID          string
	InstallationID string
	PrivateKeyPEM  string
	PrivateKeyFile string
	BaseURL        string
	HTTPClient     *http.Client
}

// AppInstallationTokenProvider exchanges a GitHub App JWT for installation
// access tokens and caches them until shortly before expiry.
type AppInstallationTokenProvider struct {
	appID          string
	installationID string
	privateKey     *rsa.PrivateKey
	baseURL        string
	http           *http.Client

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// NewAppInstallationTokenProvider creates a provider for GitHub App
// installation access tokens.
func NewAppInstallationTokenProvider(cfg *AppConfig) (*AppInstallationTokenProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("GitHub App config is required")
	}
	keyPEM := strings.TrimSpace(cfg.PrivateKeyPEM)
	if keyPEM == "" && cfg.PrivateKeyFile != "" {
		data, err := os.ReadFile(cfg.PrivateKeyFile)
		if err != nil {
			return nil, fmt.Errorf("read GitHub App private key: %w", err)
		}
		keyPEM = string(data)
	}
	if cfg.AppID == "" || cfg.InstallationID == "" || keyPEM == "" {
		return nil, fmt.Errorf("GitHub App ID, installation ID, and private key are required")
	}
	key, err := parseRSAPrivateKey([]byte(keyPEM))
	if err != nil {
		return nil, err
	}
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &AppInstallationTokenProvider{
		appID:          cfg.AppID,
		installationID: cfg.InstallationID,
		privateKey:     key,
		baseURL:        baseURL,
		http:           httpClient,
	}, nil
}

// Token returns a cached installation token, refreshing when it is near expiry.
func (p *AppInstallationTokenProvider) Token(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.token != "" && time.Now().Before(p.expiresAt.Add(-installationTokenRefreshSkew)) {
		return p.token, nil
	}

	jwt, err := p.jwt(time.Now())
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/app/installations/"+p.installationID+"/access_tokens", http.NoBody)
	if err != nil {
		return "", fmt.Errorf("create installation token request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := p.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("request installation token: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read installation token response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("GitHub App installation token error (status %d): %s", resp.StatusCode, string(data))
	}
	var parsed struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("parse installation token response: %w", err)
	}
	if parsed.Token == "" {
		return "", fmt.Errorf("installation token response missing token")
	}
	p.token = parsed.Token
	p.expiresAt = parsed.ExpiresAt
	return p.token, nil
}

func (p *AppInstallationTokenProvider) jwt(now time.Time) (string, error) {
	claims := map[string]int64{
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(10 * time.Minute).Unix(),
	}
	if n, err := strconv.ParseInt(p.appID, 10, 64); err == nil {
		claims["iss"] = n
	} else {
		return "", fmt.Errorf("parse GitHub App ID: %w", err)
	}
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	headerData, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimData, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := base64.RawURLEncoding.EncodeToString(headerData) + "." + base64.RawURLEncoding.EncodeToString(claimData)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, p.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign GitHub App JWT: %w", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func parseRSAPrivateKey(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(bytes.TrimSpace(data))
	if block == nil {
		return nil, fmt.Errorf("parse GitHub App private key: missing PEM block")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub App private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("parse GitHub App private key: expected RSA key")
	}
	return key, nil
}

func tokenProviderFromEnv(baseURL string, httpClient *http.Client) TokenProvider {
	appID := strings.TrimSpace(os.Getenv("GITHUB_APP_ID"))
	installationID := strings.TrimSpace(os.Getenv("GITHUB_APP_INSTALLATION_ID"))
	privateKey := os.Getenv("GITHUB_APP_PRIVATE_KEY")
	privateKeyFile := strings.TrimSpace(os.Getenv("GITHUB_APP_PRIVATE_KEY_FILE"))
	if appID != "" && installationID != "" && (strings.TrimSpace(privateKey) != "" || privateKeyFile != "") {
		provider, err := NewAppInstallationTokenProvider(&AppConfig{
			AppID:          appID,
			InstallationID: installationID,
			PrivateKeyPEM:  privateKey,
			PrivateKeyFile: privateKeyFile,
			BaseURL:        baseURL,
			HTTPClient:     httpClient,
		})
		if err != nil {
			return errorTokenProvider{err: err}
		}
		return provider
	}
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	return staticTokenProvider{token: token}
}

// TokenFromEnv returns the configured GitHub bearer token. GitHub App
// installation tokens take precedence when app credentials are complete.
func TokenFromEnv(ctx context.Context) (string, error) {
	baseURL := os.Getenv("GITHUB_API_URL")
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	return tokenProviderFromEnv(baseURL, &http.Client{Timeout: 30 * time.Second}).Token(ctx)
}
