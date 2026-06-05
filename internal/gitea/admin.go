package gitea

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// CreateUserRequest is the payload for creating a Gitea user via Admin API.
type CreateUserRequest struct {
	LoginName          string `json:"login_name"`
	Username           string `json:"username"`
	Email              string `json:"email"`
	Password           string `json:"password"`
	SendNotify         bool   `json:"send_notify"`
	MustChangePassword bool   `json:"must_change_password"`
}

// UserResponse represents a Gitea user.
type UserResponse struct {
	ID    int    `json:"id"`
	Login string `json:"login"`
	Email string `json:"email"`
}

// TokenResponse represents a created API token.
type TokenResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	SHA1 string `json:"sha1"`
}

// AdminCreateUser creates a new user via the Admin API.
func (c *Client) AdminCreateUser(req CreateUserRequest) (*UserResponse, error) {
	body, err := c.do("POST", "/admin/users", req)
	if err != nil {
		return nil, fmt.Errorf("admin create user: %w", err)
	}

	var user UserResponse
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("unmarshal user: %w", err)
	}
	return &user, nil
}

// AdminCreateToken creates an API token for the given user via Admin API.
func (c *Client) AdminCreateToken(username, tokenName string) (*TokenResponse, error) {
	body, err := c.do("POST", fmt.Sprintf("/admin/users/%s/tokens", username), map[string]string{
		"name": tokenName,
	})
	if err != nil {
		return nil, fmt.Errorf("admin create token: %w", err)
	}

	var token TokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("unmarshal token: %w", err)
	}
	return &token, nil
}

// AdminDeleteUser deletes a user via the Admin API.
func (c *Client) AdminDeleteUser(username string) error {
	_, err := c.do("DELETE", fmt.Sprintf("/admin/users/%s", username), nil)
	if err != nil {
		return fmt.Errorf("admin delete user: %w", err)
	}
	return nil
}

// AdminAddCollaborator adds a user as a collaborator to a repository.
func (c *Client) AdminAddCollaborator(owner, repo, username string) error {
	_, err := c.do("PUT", fmt.Sprintf("/repos/%s/%s/collaborators/%s", owner, repo, username),
		map[string]string{"permission": "write"})
	if err != nil {
		return fmt.Errorf("add collaborator: %w", err)
	}
	return nil
}

// CreateTokenWithCredentials creates a token using the user's own credentials.
// This is required for Gitea 1.26+ where admin token creation is not supported.
func (c *Client) CreateTokenWithCredentials(username, password, tokenName string) (*TokenResponse, error) {
	url := fmt.Sprintf("%s/api/v1/users/%s/tokens", c.BaseURL, username)

	reqBody := map[string]interface{}{
		"name":   tokenName,
		"scopes": []string{"all"},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.SetBasicAuth(username, password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var token TokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("unmarshal token: %w", err)
	}
	return &token, nil
}
