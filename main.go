package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitea-agent-gateway/internal/agents"
	"gitea-agent-gateway/internal/api"
	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/dispatcher"
	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/store"
	"gitea-agent-gateway/internal/webhook"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("[FATAL] Failed to load config: %v", err)
	}

	// Open database
	db, err := store.Open(cfg.Database.Path)
	if err != nil {
		log.Fatalf("[FATAL] Failed to open database: %v", err)
	}
	defer db.Close()
	log.Printf("[INFO] Database opened: %s", cfg.Database.Path)

	// Ensure workspace directory exists
	if err := os.MkdirAll(cfg.Workspace.BaseDir, 0755); err != nil {
		log.Fatalf("[FATAL] Failed to create workspace dir: %v", err)
	}

	// Initialize LLM registry
	llmRegistry := llm.NewRegistry(&cfg.LLM)

	// Initialize dispatcher (Router + TaskQueue + Executor)
	d := dispatcher.NewDispatcher(db, &cfg.Gitea, &cfg.Dispatcher, llmRegistry)

	// Start dispatcher (loads pending tasks and starts workers)
	if err := d.Start(); err != nil {
		log.Fatalf("[FATAL] Failed to start dispatcher: %v", err)
	}

	// Build HTTP server
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","version":"0.1.0"}`)
	})

	// Webhook handler - wired to dispatcher
	webhookHandler := webhook.NewHandler(&cfg.Gitea, db.DB, d.HandleEvent)
	mux.Handle("POST /webhook/gitea", webhookHandler)

	// Management API
	manager := agents.NewManager(db, &cfg.Gitea)
	apiHandler := api.NewHandler(db, manager)
	apiHandler.RegisterRoutes(mux)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[INFO] Server starting on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Server failed: %v", err)
		}
	}()

	<-done
	log.Println("[INFO] Server shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("[FATAL] Server forced to shutdown: %v", err)
	}

	log.Println("[INFO] Server exited cleanly")
}
