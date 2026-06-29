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

// GetCheckRuns retrieves check runs for a given Git ref.
func (c *Client) GetCheckRuns(ref string) ([]CheckRun, error) {
	path := fmt.Sprintf("/%s/commits/%s/check-runs?per_page=50", c.RepoPath(), ref)

	var resp struct {
		CheckRuns []CheckRun `json:"check_runs"`
	}
	if err := c.doJSON("GET", path, nil, &resp); err != nil {
		return nil, fmt.Errorf("get check runs: %w", err)
	}

	return resp.CheckRuns, nil
}

// GetCheckSuites retrieves check suites for a given Git ref.
func (c *Client) GetCheckSuites(ref string) ([]CheckSuite, error) {
	path := fmt.Sprintf("/%s/commits/%s/check-suites", c.RepoPath(), ref)

	var resp struct {
		CheckSuites []CheckSuite `json:"check_suites"`
	}
	if err := c.doJSON("GET", path, nil, &resp); err != nil {
		return nil, fmt.Errorf("get check suites: %w", err)
	}

	return resp.CheckSuites, nil
}

// GetCheckRunAnnotations retrieves annotations for a check run.
func (c *Client) GetCheckRunAnnotations(checkRunID int) (string, error) {
	path := fmt.Sprintf("/%s/check-runs/%d/annotations", c.RepoPath(), checkRunID)

	var annotations []struct {
		Path            string `json:"path"`
		Message         string `json:"message"`
		AnnotationLevel string `json:"annotation_level"`
	}
	if err := c.doJSON("GET", path, nil, &annotations); err != nil {
		return "", fmt.Errorf("get annotations: %w", err)
	}

	output := ""
	for _, a := range annotations {
		output += fmt.Sprintf("[%s] %s: %s\n", a.AnnotationLevel, a.Path, a.Message)
	}
	return output, nil
}

// GetWorkflowRunLogs retrieves the logs for a workflow run.
func (c *Client) GetWorkflowRunLogs(runID int) (string, error) {
	path := fmt.Sprintf("/%s/actions/runs/%d/logs", c.RepoPath(), runID)

	data, err := c.do("GET", path, nil)
	if err != nil {
		return "", fmt.Errorf("get workflow logs: %w", err)
	}

	return string(data), nil
}
