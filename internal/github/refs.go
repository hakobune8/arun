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

import "fmt"

// GitRef describes a Git reference returned by GitHub.
type GitRef struct {
	Ref    string `json:"ref"`
	Object struct {
		SHA string `json:"sha"`
	} `json:"object"`
}

// CreateRefRequest contains parameters for creating a Git reference.
type CreateRefRequest struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

// GetBranchSHA returns the head SHA for a branch.
func (c *Client) GetBranchSHA(branch string) (string, error) {
	var ref GitRef
	path := fmt.Sprintf("/%s/git/ref/heads/%s", c.RepoPath(), branch)
	if err := c.doJSON("GET", path, nil, &ref); err != nil {
		return "", fmt.Errorf("get branch ref: %w", err)
	}
	if ref.Object.SHA == "" {
		return "", fmt.Errorf("get branch ref: response missing SHA")
	}
	return ref.Object.SHA, nil
}

// CreateBranch creates a branch from an existing commit SHA.
func (c *Client) CreateBranch(branch, sha string) error {
	req := CreateRefRequest{Ref: "refs/heads/" + branch, SHA: sha}
	path := fmt.Sprintf("/%s/git/refs", c.RepoPath())
	if err := c.doJSON("POST", path, req, nil); err != nil {
		return fmt.Errorf("create branch ref: %w", err)
	}
	return nil
}

// DeleteBranch deletes a branch reference.
func (c *Client) DeleteBranch(branch string) error {
	path := fmt.Sprintf("/%s/git/refs/heads/%s", c.RepoPath(), branch)
	if err := c.doJSON("DELETE", path, nil, nil); err != nil {
		return fmt.Errorf("delete branch ref: %w", err)
	}
	return nil
}
