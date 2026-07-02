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

// ListRepositories lists repositories available to the configured GitHub token.
func (c *Client) ListRepositories() ([]RepositorySummary, error) {
	if repos, err := c.listInstallationRepositories(); err == nil {
		return repos, nil
	}
	return c.listUserRepositories()
}

func (c *Client) listInstallationRepositories() ([]RepositorySummary, error) {
	var repos []RepositorySummary
	for page := 1; ; page++ {
		var installation struct {
			Repositories []RepositorySummary `json:"repositories"`
		}
		path := fmt.Sprintf("/installation/repositories?per_page=100&page=%d", page)
		if err := c.doJSON("GET", path, nil, &installation); err != nil {
			return nil, err
		}
		repos = append(repos, installation.Repositories...)
		if len(installation.Repositories) < 100 {
			return repos, nil
		}
	}
}

func (c *Client) listUserRepositories() ([]RepositorySummary, error) {
	var repos []RepositorySummary
	for page := 1; ; page++ {
		var pageRepos []RepositorySummary
		path := fmt.Sprintf("/user/repos?per_page=100&page=%d&sort=updated&affiliation=owner,collaborator,organization_member", page)
		if err := c.doJSON("GET", path, nil, &pageRepos); err != nil {
			return nil, err
		}
		repos = append(repos, pageRepos...)
		if len(pageRepos) < 100 {
			return repos, nil
		}
	}
}
