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

// Package github provides a client for the GitHub REST API.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Client is a GitHub API client for interacting with issues, PRs, and checks.
type Client struct {
	BaseURL       string
	Token         string
	RepoOwner     string
	RepoName      string
	http          *http.Client
	tokenProvider TokenProvider
}

// NewClient creates a new GitHub API client for the given repository.
func NewClient(repoOwner, repoName string) *Client {
	baseURL := os.Getenv("GITHUB_API_URL")
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}
	tokenProvider := tokenProviderFromEnv(baseURL, httpClient)
	token, _ := tokenProvider.Token(context.Background()) //nolint:errcheck // best-effort compatibility field

	return &Client{
		BaseURL:       baseURL,
		Token:         token,
		RepoOwner:     repoOwner,
		RepoName:      repoName,
		http:          httpClient,
		tokenProvider: tokenProvider,
	}
}

// WithToken returns the client configured to use an explicit bearer token.
func (c *Client) WithToken(token string) *Client {
	c.Token = strings.TrimSpace(token)
	c.tokenProvider = staticTokenProvider{token: c.Token}
	return c
}

func (c *Client) do(method, path string, body io.Reader) ([]byte, error) {
	var bodyData []byte
	if body != nil {
		data, err := io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("read request body: %w", err)
		}
		bodyData = data
	}

	url := c.BaseURL + path
	req, err := http.NewRequest(method, url, bytes.NewReader(bodyData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	token := c.Token
	if c.tokenProvider != nil {
		next, err := c.tokenProvider.Token(req.Context())
		if err != nil {
			return nil, fmt.Errorf("get GitHub token: %w", err)
		}
		token = next
		c.Token = next
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if bodyData != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		if resp.StatusCode == http.StatusUnauthorized && token != "" {
			return c.doWithAuthorization(method, url, bodyData, "token "+token)
		}
		return nil, fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (c *Client) doWithAuthorization(method, url string, bodyData []byte, authorization string) ([]byte, error) {
	req, err := http.NewRequest(method, url, bytes.NewReader(bodyData))
	if err != nil {
		return nil, fmt.Errorf("create retry request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", authorization)
	if bodyData != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http retry request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read retry response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

func (c *Client) doJSON(method, path string, reqBody, respBody interface{}) error {
	var bodyReader io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = strings.NewReader(string(data))
	}

	data, err := c.do(method, path, bodyReader)
	if err != nil {
		return err
	}

	if respBody != nil && len(data) > 0 {
		if err := json.Unmarshal(data, respBody); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}

	return nil
}

// RepoPath returns the GitHub API path for the repository.
func (c *Client) RepoPath() string {
	return fmt.Sprintf("repos/%s/%s", c.RepoOwner, c.RepoName)
}

// Repo returns the "owner/name" string for the repository.
func (c *Client) Repo() string {
	return fmt.Sprintf("%s/%s", c.RepoOwner, c.RepoName)
}
