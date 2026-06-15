package gitea

// User represents a Gitea user.
type User struct {
	ID    int    `json:"id"`
	Login string `json:"login"`
	Name  string `json:"full_name"`
}

// Issue represents a Gitea issue.
type Issue struct {
	ID      int    `json:"id"`
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	State   string `json:"state"`
	User    User   `json:"user"`
	HTMLURL string `json:"html_url"`
}

// PullRequest represents a Gitea pull request.
type PullRequest struct {
	ID      int    `json:"id"`
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	State   string `json:"state"`
	User    User   `json:"user"`
	Head    string `json:"head"`
	Base    string `json:"base"`
	HTMLURL string `json:"html_url"`
}
