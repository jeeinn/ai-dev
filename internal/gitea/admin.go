package gitea

import (
	"encoding/json"
	"fmt"
)

// CreateUserRequest is the payload for creating a Gitea user via Admin API.
type CreateUserRequest struct {
	LoginName string `json:"login_name"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	SendNotify bool  `json:"send_notify"`
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
