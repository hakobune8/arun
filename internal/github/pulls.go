package github

import "fmt"

type CreatePRRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Head  string `json:"head"`
	Base  string `json:"base"`
}

type createPRResponse struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	HTMLURL string `json:"html_url"`
	State   string `json:"state"`
}

func (c *Client) CreatePR(req CreatePRRequest) (*PullRequest, error) {
	path := fmt.Sprintf("/%s/pulls", c.RepoPath())

	var resp createPRResponse
	if err := c.doJSON("POST", path, req, &resp); err != nil {
		return nil, fmt.Errorf("create PR: %w", err)
	}

	return &PullRequest{
		Number:  resp.Number,
		Title:   resp.Title,
		HTMLURL: resp.HTMLURL,
		State:   resp.State,
		Head:    req.Head,
		Base:    req.Base,
	}, nil
}

func (c *Client) ListPRs(state string) ([]PullRequest, error) {
	if state == "" {
		state = "open"
	}
	path := fmt.Sprintf("/%s/pulls?state=%s", c.RepoPath(), state)

	var prs []struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		HTMLURL string `json:"html_url"`
		State   string `json:"state"`
		Head    struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	}
	if err := c.doJSON("GET", path, nil, &prs); err != nil {
		return nil, fmt.Errorf("list PRs: %w", err)
	}

	var result []PullRequest
	for _, pr := range prs {
		result = append(result, PullRequest{
			Number:  pr.Number,
			Title:   pr.Title,
			HTMLURL: pr.HTMLURL,
			State:   pr.State,
			Head:    pr.Head.Ref,
			Base:    pr.Base.Ref,
		})
	}
	return result, nil
}
