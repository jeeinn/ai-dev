package gitea

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	ID      int    `json:"id"`
	Number  int    `json:"number"`
	Title   string `json:"title"`
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

// PRDiff returns the diff of a pull request.
func (c *Client) PRDiff(owner, repo string, prID int) (string, error) {
	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d.diff", c.BaseURL, owner, repo, prID), nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+c.Token)

	respBody, status, err := c.execute(req, nil)
	if err != nil {
		return "", err
	}
	if status >= 400 {
		return "", fmt.Errorf("API error %d: %s", status, string(respBody))
	}
	return string(respBody), nil
}

// PRFiles returns the list of files changed in a pull request.
type PRFile struct {
	Filename  string `json:"filename"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Changes   int    `json:"changes"`
	Patch     string `json:"patch,omitempty"`
}

func (c *Client) PRFiles(owner, repo string, prID int) ([]PRFile, error) {
	body, err := c.do("GET", fmt.Sprintf("/repos/%s/%s/pulls/%d/files", owner, repo, prID), nil)
	if err != nil {
		return nil, fmt.Errorf("PR files: %w", err)
	}

	var files []PRFile
	if err := json.Unmarshal(body, &files); err != nil {
		return nil, fmt.Errorf("unmarshal PR files: %w", err)
	}
	return files, nil
}

// IssueComment represents a comment on an issue or PR.
type IssueComment struct {
	ID      int    `json:"id"`
	Body    string `json:"body"`
	User    User   `json:"user"`
	Created string `json:"created_at"`
	Updated string `json:"updated_at"`
}

// IssueComments returns the comments on an issue or PR.
func (c *Client) IssueComments(owner, repo string, issueID int) ([]IssueComment, error) {
	body, err := c.do("GET", fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, issueID), nil)
	if err != nil {
		return nil, fmt.Errorf("issue comments: %w", err)
	}

	var comments []IssueComment
	if err := json.Unmarshal(body, &comments); err != nil {
		return nil, fmt.Errorf("unmarshal comments: %w", err)
	}
	return comments, nil
}
