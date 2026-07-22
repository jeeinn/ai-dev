package gitea

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueUnassignRemovesNamedAssignees(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v1/repos/owner/repo/issues/7/assignees", r.URL.Path)
		assert.Equal(t, "token test-token", r.Header.Get("Authorization"))

		var body map[string][]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, []string{"code-analyzer"}, body["assignees"])

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	err := client.IssueUnassign("owner", "repo", 7, "code-analyzer")
	require.NoError(t, err)
}

func TestIssueUnassignRequiresUsername(t *testing.T) {
	client := NewClient("http://example.invalid", "test-token")
	err := client.IssueUnassign("owner", "repo", 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one username")
}
