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

package github

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

// PutFileRequest contains the parameters for creating or updating a file.
type PutFileRequest struct {
	Message string `json:"message"`
	Content string `json:"content"`
	Branch  string `json:"branch,omitempty"`
}

// PutFile creates or updates a repository file on the requested branch.
func (c *Client) PutFile(path string, req PutFileRequest) error {
	path = cleanContentPath(path)
	if path == "" {
		return fmt.Errorf("put file: path is required")
	}
	if strings.TrimSpace(req.Message) == "" {
		return fmt.Errorf("put file: message is required")
	}
	req.Content = base64.StdEncoding.EncodeToString([]byte(req.Content))
	apiPath := fmt.Sprintf("/%s/contents/%s", c.RepoPath(), path)
	if err := c.doJSON("PUT", apiPath, req, nil); err != nil {
		return fmt.Errorf("put file: %w", err)
	}
	return nil
}

func cleanContentPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		escaped = append(escaped, url.PathEscape(part))
	}
	return strings.Join(escaped, "/")
}
