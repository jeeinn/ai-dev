package gitea

import (
	"encoding/json"
	"fmt"
)

// CreatePRRequest is the payload for creating a pull request.
type CreatePRRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Head  string `json:"head"`
	Base  string `json:"base"`
}

// PRResponse represents a Gitea pull request.
type PRResponse struct {
	ID     int    `json:"id"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	HTMLURL string `json:"html_url"`
}

// CreatePR creates a new pull request.
func (c *Client) CreatePR(owner, repo string, req CreatePRRequest) (*PRResponse, error) {
	body, err := c.do("POST", fmt.Sprintf("/repos/%s/%s/pulls", owner, repo), req)
	if err != nil {
		return nil, fmt.Errorf("create PR: %w", err)
	}

	var pr PRResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, fmt.Errorf("unmarshal PR: %w", err)
	}
	return &pr, nil
}

// PRComment posts a comment on the given pull request.
func (c *Client) PRComment(owner, repo string, prID int, body string) error {
	_, err := c.do("POST", fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, prID),
		map[string]string{"body": body})
	if err != nil {
		return fmt.Errorf("PR comment: %w", err)
	}
	return nil
}

// PRGet returns the pull request details.
func (c *Client) PRGet(owner, repo string, prID int) (map[string]interface{}, error) {
	body, err := c.do("GET", fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, prID), nil)
	if err != nil {
		return nil, fmt.Errorf("PR get: %w", err)
	}

	var pr map[string]interface{}
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, fmt.Errorf("unmarshal PR: %w", err)
	}
	return pr, nil
}
