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
	dbPath := "./data/gateway.db"
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Insert agent
	_, err = db.Exec(`INSERT OR REPLACE INTO agents (id, name, gitea_username, gitea_token, provider, model, max_tokens, temperature, system_prompt, user_template, status)
		VALUES (1, 'test-agent', 'agent', '4212aa8837d31af337504310a6f8de2390a52348', 'deepseek', 'deepseek-chat', 2048, 0.3, 'You are a helpful AI assistant.', '', 'active')`)
	if err != nil {
		log.Fatalf("Failed to insert agent: %v", err)
	}
	fmt.Println("Agent created: id=1")

	// Insert route for issues assigned
	_, err = db.Exec(`INSERT OR REPLACE INTO routes (id, event, action, label, assignee, mention, agent_id, priority)
		VALUES (1, 'issues', 'assigned', '', '', '', 1, 10)`)
	if err != nil {
		log.Fatalf("Failed to insert route: %v", err)
	}
	fmt.Println("Route created: id=1 (issues/assigned)")

	// Insert route for issue comments mentioning agent
	_, err = db.Exec(`INSERT OR REPLACE INTO routes (id, event, action, label, assignee, mention, agent_id, priority)
		VALUES (2, 'issue_comment', '', '', '', 'agent', 1, 20)`)
	if err != nil {
		log.Fatalf("Failed to insert route: %v", err)
	}
	fmt.Println("Route created: id=2 (issue_comment/@agent)")

	fmt.Println("Test setup complete!")
}
