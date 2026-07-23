//go:build ignore

package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := "./data/matea.db"
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Insert analyze agent
	_, err = db.Exec(`INSERT OR REPLACE INTO agents (id, name, gitea_username, gitea_token, provider, model, max_output_tokens, max_input_tokens, temperature, timeout, system_prompt, user_template, role, status)
		VALUES (1, 'analyze-agent', 'analyze-agent', '4212aa8837d31af337504310a6f8de2390a52348', 'deepseek', 'deepseek-chat', 2048, 8192, 0.3, '5m', 'You are a helpful AI assistant.', '', 'analyze', 'active')`)
	if err != nil {
		log.Fatalf("Failed to insert analyze agent: %v", err)
	}
	fmt.Println("Agent created: id=1 (analyze)")

	// Insert coder agent
	_, err = db.Exec(`INSERT OR REPLACE INTO agents (id, name, gitea_username, gitea_token, provider, model, max_output_tokens, max_input_tokens, temperature, timeout, system_prompt, user_template, role, status)
		VALUES (2, 'coder-agent', 'coder-agent', '4212aa8837d31af337504310a6f8de2390a52348', 'deepseek', 'deepseek-chat', 4096, 8192, 0.3, '5m', 'You are a coding AI assistant.', '', 'coder', 'active')`)
	if err != nil {
		log.Fatalf("Failed to insert coder agent: %v", err)
	}
	fmt.Println("Agent created: id=2 (coder)")

	// Insert review agent
	_, err = db.Exec(`INSERT OR REPLACE INTO agents (id, name, gitea_username, gitea_token, provider, model, max_output_tokens, max_input_tokens, temperature, timeout, system_prompt, user_template, role, status)
		VALUES (3, 'review-agent', 'review-agent', '4212aa8837d31af337504310a6f8de2390a52348', 'deepseek', 'deepseek-chat', 4096, 8192, 0.3, '5m', 'You are a code review AI assistant.', '', 'review', 'active')`)
	if err != nil {
		log.Fatalf("Failed to insert review agent: %v", err)
	}
	fmt.Println("Agent created: id=3 (review)")

	fmt.Println("\nTest setup complete! (v2 Assign model — no routes table)")
	fmt.Println("\nUsage:")
	fmt.Println("  Assign agents to issues by setting the Gitea assignee to the agent's gitea_username.")
	fmt.Println("  analyze-agent → analyze_issue, coder-agent → solve_issue, review-agent → review_pr")
}
