package gitea

import (
	"encoding/json"
	"fmt"
)

// CurrentUser represents the authenticated Gitea user.
type CurrentUser struct {
	ID       int    `json:"id"`
	Login    string `json:"login"`
	IsAdmin  bool   `json:"is_admin"`
	FullName string `json:"full_name"`
}

// GetCurrentUser returns the user associated with the API token.
func (c *Client) GetCurrentUser() (*CurrentUser, error) {
	body, err := c.do("GET", "/user", nil)
	if err != nil {
		return nil, fmt.Errorf("get current user: %w", err)
	}

	var user CurrentUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("unmarshal user: %w", err)
	}
	return &user, nil
}

// ConnectionTestResult summarizes a Gitea connectivity check.
type ConnectionTestResult struct {
	OK        bool   `json:"ok"`
	Username  string `json:"username,omitempty"`
	IsAdmin   bool   `json:"is_admin,omitempty"`
	RepoCount int    `json:"repo_count,omitempty"`
	Message   string `json:"message"`
}

// TestConnection verifies URL and token by calling the Gitea API.
func (c *Client) TestConnection() (*ConnectionTestResult, error) {
	if c.BaseURL == "" {
		return &ConnectionTestResult{Message: "Gitea 地址不能为空"}, nil
	}
	if c.Token == "" {
		return &ConnectionTestResult{Message: "管理员 Token 不能为空"}, nil
	}

	user, err := c.GetCurrentUser()
	if err != nil {
		return &ConnectionTestResult{Message: err.Error()}, nil
	}

	repos, err := c.ListRepos()
	if err != nil {
		return &ConnectionTestResult{
			Username: user.Login,
			IsAdmin:  user.IsAdmin,
			Message:  fmt.Sprintf("Token 有效，但列出仓库失败: %v", err),
		}, nil
	}

	msg := fmt.Sprintf("连接成功，可见 %d 个仓库", len(repos))
	if !user.IsAdmin {
		msg += "；警告：当前用户非管理员，创建 Agent 需要 write:admin 权限"
	}

	return &ConnectionTestResult{
		OK:        true,
		Username:  user.Login,
		IsAdmin:   user.IsAdmin,
		RepoCount: len(repos),
		Message:   msg,
	}, nil
}
