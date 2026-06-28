package github

import "fmt"

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

func (c *Client) GetCheckRunAnnotations(checkRunID int) (string, error) {
	path := fmt.Sprintf("/%s/check-runs/%d/annotations", c.RepoPath(), checkRunID)

	var annotations []struct {
		Path        string `json:"path"`
		Message     string `json:"message"`
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

func (c *Client) GetWorkflowRunLogs(runID int) (string, error) {
	path := fmt.Sprintf("/%s/actions/runs/%d/logs", c.RepoPath(), runID)

	data, err := c.do("GET", path, nil)
	if err != nil {
		return "", fmt.Errorf("get workflow logs: %w", err)
	}

	return string(data), nil
}
