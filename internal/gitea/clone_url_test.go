package gitea

import "testing"

func TestAuthenticatedCloneURL(t *testing.T) {
	tests := []struct {
		name     string
		cloneURL string
		username string
		token    string
		want     string
		wantErr  bool
	}{
		{
			name:     "no token returns original",
			cloneURL: "http://gitea.example.com/owner/repo.git",
			want:     "http://gitea.example.com/owner/repo.git",
		},
		{
			name:     "token only",
			cloneURL: "http://gitea.example.com/owner/repo.git",
			token:    "secret-token",
			want:     "http://secret-token@gitea.example.com/owner/repo.git",
		},
		{
			name:     "username and token",
			cloneURL: "http://gitea.example.com/owner/repo.git",
			username: "ai-agent",
			token:    "secret-token",
			want:     "http://ai-agent:secret-token@gitea.example.com/owner/repo.git",
		},
		{
			name:     "https url",
			cloneURL: "https://gitea.example.com/owner/repo.git",
			username: "ai-agent",
			token:    "secret-token",
			want:     "https://ai-agent:secret-token@gitea.example.com/owner/repo.git",
		},
		{
			name:     "invalid url",
			cloneURL: "://bad",
			token:    "secret-token",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AuthenticatedCloneURL(tt.cloneURL, tt.username, tt.token)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("AuthenticatedCloneURL() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("AuthenticatedCloneURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRedactCloneURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain url unchanged",
			in:   "http://gitea.example.com/owner/repo.git",
			want: "http://gitea.example.com/owner/repo.git",
		},
		{
			name: "redact username and token",
			in:   "http://ai-agent:secret-token@gitea.example.com/owner/repo.git",
			want: "http://%2A%2A%2A:%2A%2A%2A@gitea.example.com/owner/repo.git",
		},
		{
			name: "redact token only",
			in:   "http://secret-token@gitea.example.com/owner/repo.git",
			want: "http://%2A%2A%2A@gitea.example.com/owner/repo.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactCloneURL(tt.in)
			if got != tt.want {
				t.Errorf("RedactCloneURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveDefaultBranch(t *testing.T) {
	if got := ResolveDefaultBranch("develop"); got != "develop" {
		t.Errorf("ResolveDefaultBranch(develop) = %q", got)
	}
	if got := ResolveDefaultBranch(""); got != "main" {
		t.Errorf("ResolveDefaultBranch('') = %q, want main", got)
	}
}
