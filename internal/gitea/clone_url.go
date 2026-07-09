package gitea

import (
	"fmt"
	"net/url"
	"strings"
)

// AuthenticatedCloneURL embeds credentials into a clone URL for git operations.
// When token is empty, the original URL is returned unchanged.
func AuthenticatedCloneURL(cloneURL, username, token string) (string, error) {
	if token == "" {
		return cloneURL, nil
	}
	u, err := url.Parse(cloneURL)
	if err != nil {
		return "", fmt.Errorf("parse clone url: %w", err)
	}
	if username != "" {
		u.User = url.UserPassword(username, token)
	} else {
		u.User = url.User(token)
	}
	return u.String(), nil
}

// RedactCloneURL returns a clone URL with credentials masked for logging.
func RedactCloneURL(cloneURL string) string {
	u, err := url.Parse(cloneURL)
	if err != nil || u.User == nil {
		return cloneURL
	}
	if _, hasPassword := u.User.Password(); hasPassword {
		u.User = url.UserPassword("***", "***")
	} else {
		u.User = url.User("***")
	}
	return u.String()
}

// ResolveDefaultBranch returns defaultBranch when set, otherwise "main".
func ResolveDefaultBranch(defaultBranch string) string {
	if strings.TrimSpace(defaultBranch) != "" {
		return defaultBranch
	}
	return "main"
}
