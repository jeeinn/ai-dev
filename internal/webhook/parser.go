package webhook

import (
	"encoding/json"
	"log"
)

// WebhookEvent represents the parsed Gitea webhook event.
type WebhookEvent struct {
	DeliveryID string       `json:"-"`
	Event      string       `json:"-"`
	Action     string       `json:"action"`
	Repo       Repository   `json:"repository"`
	Issue      *Issue       `json:"issue,omitempty"`
	PR         *PullRequest `json:"pull_request,omitempty"`
	Comment    *Comment     `json:"comment,omitempty"`
	Sender     User         `json:"sender"`
}

type Repository struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	FullName string `json:"full_name"`
	Owner     User   `json:"owner"`
	CloneURL  string `json:"clone_url"`
	SSHURL    string `json:"ssh_url"`
	DefaultBranch string `json:"default_branch"`
}

type Issue struct {
	ID        int    `json:"id"`
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	State     string `json:"state"`
	User      User   `json:"user"`
	Assignees []User `json:"assignees"`
	Labels    []Label `json:"labels"`
	HTMLURL   string `json:"html_url"`
}

type PullRequest struct {
	ID        int    `json:"id"`
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	State     string `json:"state"`
	User      User   `json:"user"`
	Head      Branch `json:"head"`
	Base      Branch `json:"base"`
	HTMLURL   string `json:"html_url"`
}

type Branch struct {
	Ref  string     `json:"ref"`
	Repo Repository `json:"repo"`
}

type Comment struct {
	ID      int    `json:"id"`
	Body    string `json:"body"`
	User    User   `json:"user"`
	HTMLURL string `json:"html_url"`
}

type User struct {
	ID    int    `json:"id"`
	Login string `json:"login"`
}

type Label struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ParseEvent parses the webhook payload based on the event type.
func ParseEvent(eventType, deliveryID string, payload []byte) (*WebhookEvent, error) {
	var evt WebhookEvent
	if err := json.Unmarshal(payload, &evt); err != nil {
		return nil, err
	}
	evt.DeliveryID = deliveryID
	evt.Event = eventType

	log.Printf("[DEBUG] Parsed event: type=%s action=%s repo=%s sender=%s",
		eventType, evt.Action, evt.Repo.FullName, evt.Sender.Login)

	return &evt, nil
}

// HasLabel checks if the issue has the given label.
func (evt *WebhookEvent) HasLabel(label string) bool {
	if evt.Issue == nil {
		return false
	}
	for _, l := range evt.Issue.Labels {
		if l.Name == label {
			return true
		}
	}
	return false
}

// HasAssignee checks if the issue is assigned to the given user.
func (evt *WebhookEvent) HasAssignee(username string) bool {
	if evt.Issue == nil {
		return false
	}
	for _, a := range evt.Issue.Assignees {
		if a.Login == username {
			return true
		}
	}
	return false
}

// HasMention checks if the comment body mentions the given username.
func (evt *WebhookEvent) HasMention(username string) bool {
	if evt.Comment == nil {
		return false
	}
	mention := "@" + username
	return len(evt.Comment.Body) > 0 && contains(evt.Comment.Body, mention)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
