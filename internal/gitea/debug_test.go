package gitea

import (
	"strings"
	"testing"
)

func TestSanitizeBodyForLogRedactsPassword(t *testing.T) {
	body := []byte(`{"login_name":"agent","password":"secret123","email":"a@b.c"}`)
	got := sanitizeBodyForLog(body)
	if got == "" {
		t.Fatal("expected sanitized body")
	}
	if strings.Contains(got, "secret123") {
		t.Fatalf("password leaked in log preview: %s", got)
	}
	if !strings.Contains(got, "***") {
		t.Fatalf("expected redacted marker: %s", got)
	}
}