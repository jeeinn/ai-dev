package gitea

import (
	"encoding/json"
	"fmt"
)

// RepoInfo represents basic repository information.
type RepoInfo struct {
	DefaultBranch string `json:"default_branch"`
	Language      string `json:"language"`
	CloneURL      string `json:"clone_url"`
}

// GetRepo returns basic repository information.
func (c *Client) GetRepo(owner, repo string) (*RepoInfo, error) {
	body, err := c.do("GET", fmt.Sprintf("/repos/%s/%s", owner, repo), nil)
	if err != nil {
		return nil, fmt.Errorf("get repo: %w", err)
	}

	var info RepoInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("unmarshal repo: %w", err)
	}
	return &info, nil
}

// GetFileContent returns the content of a file in the repository.
func (c *Client) GetFileContent(owner, repo, ref, filepath string) (string, error) {
	body, err := c.do("GET", fmt.Sprintf("/repos/%s/%s/contents/%s?ref=%s", owner, repo, filepath, ref), nil)
	if err != nil {
		return "", fmt.Errorf("get file: %w", err)
	}

	var file struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(body, &file); err != nil {
		return "", fmt.Errorf("unmarshal file: %w", err)
	}
	return file.Content, nil
}
