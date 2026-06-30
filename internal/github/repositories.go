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

// ListRepositories lists repositories available to the configured GitHub token.
func (c *Client) ListRepositories() ([]RepositorySummary, error) {
	var installation struct {
		Repositories []RepositorySummary `json:"repositories"`
	}
	if err := c.doJSON("GET", "/installation/repositories?per_page=100", nil, &installation); err == nil {
		return installation.Repositories, nil
	}

	var repos []RepositorySummary
	if err := c.doJSON("GET", "/user/repos?per_page=100&sort=updated&affiliation=owner,collaborator,organization_member", nil, &repos); err != nil {
		return nil, err
	}
	return repos, nil
}
