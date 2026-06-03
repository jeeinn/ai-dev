package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gitea-agent-gateway/internal/agents"
	"gitea-agent-gateway/internal/api"
	"gitea-agent-gateway/internal/auth"
	"gitea-agent-gateway/internal/config"
	"gitea-agent-gateway/internal/dispatcher"
	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/store"
	"gitea-agent-gateway/internal/webhook"
)

//go:embed web/dist/*
var webDist embed.FS

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
	d := dispatcher.NewDispatcher(db, &cfg.Gitea, &cfg.Dispatcher, llmRegistry, &cfg.Agents)

	// Start dispatcher (loads pending tasks and starts workers)
	if err := d.Start(); err != nil {
		log.Fatalf("[FATAL] Failed to start dispatcher: %v", err)
	}

	// Initialize authentication
	jwtExpiration, err := time.ParseDuration(cfg.Auth.JWTExpiration)
	if err != nil {
		jwtExpiration = 24 * time.Hour
	}
	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, jwtExpiration)

	// Create default admin user if needed
	if err := api.EnsureDefaultAdmin(db, cfg.Auth.DefaultAdminPassword); err != nil {
		log.Printf("[WARN] Failed to create default admin: %v", err)
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

	// Serve static files (Web UI)
	webFS, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		log.Printf("[WARN] Failed to load embedded web files: %v", err)
	} else {
		fileServer := http.FileServer(http.FS(webFS))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// Try to serve static file
			path := r.URL.Path

			// Check if file exists
			f, err := webFS.Open(strings.TrimPrefix(path, "/"))
			if err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}

			// Only fallback to index.html for non-file requests
			// (requests that don't look like they're requesting a specific file)
			if !strings.Contains(path, ".") {
				r.URL.Path = "/"
				fileServer.ServeHTTP(w, r)
				return
			}

			// File not found
			http.NotFound(w, r)
		})
	}

	// Management API
	manager := agents.NewManager(db, &cfg.Gitea)
	apiHandler := api.NewHandler(db, manager, cfg)
	apiHandler.RegisterRoutes(mux)

	// Auth API
	authHandler := api.NewAuthHandler(db, jwtManager)
	authHandler.RegisterAuthRoutes(mux)

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
