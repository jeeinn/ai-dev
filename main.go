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
	"gitea-agent-gateway/internal/logging"
	"gitea-agent-gateway/internal/store"
	"gitea-agent-gateway/internal/webhook"
	"gitea-agent-gateway/internal/workflow"
)

//go:embed web/dist/*
var webDist embed.FS

// setContentType sets the correct Content-Type header based on file extension.
func setContentType(w http.ResponseWriter, path string) {
	switch {
	case strings.HasSuffix(path, ".js"):
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case strings.HasSuffix(path, ".mjs"):
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case strings.HasSuffix(path, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(path, ".html"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case strings.HasSuffix(path, ".json"):
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	case strings.HasSuffix(path, ".svg"):
		w.Header().Set("Content-Type", "image/svg+xml")
	case strings.HasSuffix(path, ".png"):
		w.Header().Set("Content-Type", "image/png")
	case strings.HasSuffix(path, ".jpg"), strings.HasSuffix(path, ".jpeg"):
		w.Header().Set("Content-Type", "image/jpeg")
	case strings.HasSuffix(path, ".ico"):
		w.Header().Set("Content-Type", "image/x-icon")
	case strings.HasSuffix(path, ".woff"):
		w.Header().Set("Content-Type", "font/woff")
	case strings.HasSuffix(path, ".woff2"):
		w.Header().Set("Content-Type", "font/woff2")
	}
}

func main() {
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("[FATAL] Failed to load config: %v", err)
	}
	logging.SetLevel(cfg.Logging.Level)
	closeLog, err := logging.SetupOutput(cfg.Logging.Path)
	if err != nil {
		log.Fatalf("[FATAL] Failed to setup logging: %v", err)
	}
	defer closeLog()

	// Open database
	db, err := store.Open(cfg.Database.Path)
	if err != nil {
		log.Fatalf("[FATAL] Failed to open database: %v", err)
	}
	defer db.Close()
	log.Printf("[INFO] Database opened: %s", cfg.Database.Path)

	// Initialize config manager (DB overrides on top of file config)
	cfgManager := config.NewConfigManager(cfg)
	cfgManager.SetStore(db)
	if err := cfgManager.MigrateLegacyConfigKeys(); err != nil {
		log.Printf("[WARN] Failed to migrate legacy config keys: %v", err)
	}
	if err := cfgManager.ApplyDBOverrides(); err != nil {
		log.Printf("[WARN] Failed to apply DB config overrides: %v", err)
	}

	// Ensure workspace directory exists
	if err := os.MkdirAll(cfg.Workspace.BaseDir, 0755); err != nil {
		log.Fatalf("[FATAL] Failed to create workspace dir: %v", err)
	}

	// Get active config (may have DB overrides)
	activeCfg := cfgManager.Get()

	// Initialize LLM registry
	llmRegistry := llm.NewRegistry(&activeCfg.LLM)
	llmRegistry.SetRateLimitBackoff(activeCfg.Dispatcher.RateLimitBackoff, activeCfg.LLM.RateLimitRetries)

	// Initialize dispatcher (Router + TaskQueue + Executor)
	d := dispatcher.NewDispatcher(db, &activeCfg.Gitea, &activeCfg.Dispatcher, llmRegistry, &activeCfg.Agents)

	// Initialize v2 workflow components
	registry := agents.NewRegistry()
	if err := registry.LoadFromDB(db); err != nil {
		log.Printf("[WARN] Failed to load agent registry: %v", err)
	}
	resolver := workflow.NewResolver(registry)
	wfMgr := workflow.NewWorkflowManager(db)
	l1Gate := workflow.NewL1Gate(db)
	sessionSvc := workflow.NewSessionService(db, activeCfg.Workspace.BaseDir)
	wfPolicy := workflow.GetPreset(activeCfg.Workflow.Preset)
	sessionCfg := &activeCfg.Session
	if sessionCfg.IdleTTL == "" {
		defaultSessionCfg := config.DefaultSessionConfig()
		sessionCfg = &defaultSessionCfg
	}
	lifecycle := workflow.NewSessionLifecycle(db, wfMgr, sessionSvc, sessionCfg, activeCfg.Workspace.BaseDir)
	d.SetWorkflowComponents(registry, resolver, wfMgr, l1Gate, sessionSvc, wfPolicy, lifecycle)

	// Start session cleanup loop (every 10 minutes)
	lifecycle.StartCleanupLoop(10 * time.Minute)

	log.Printf("[INFO] Workflow v2 components initialized (with SessionService + Lifecycle)")

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
	webhookHandler := webhook.NewHandler(&activeCfg.Gitea, db.DB, d.HandleEvent)
	mux.Handle("POST /webhook/gitea", webhookHandler)

	// Serve static files (Web UI)
	webFS, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		log.Printf("[WARN] Failed to load embedded web files: %v", err)
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			filePath := strings.TrimPrefix(path, "/")

			// Try to serve static file
			data, err := fs.ReadFile(webFS, filePath)
			if err == nil {
				setContentType(w, path)
				w.Write(data)
				return
			}

			// Only fallback to index.html for non-file requests
			if !strings.Contains(path, ".") {
				indexData, _ := fs.ReadFile(webFS, "index.html")
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(indexData)
				return
			}

			// File not found
			http.NotFound(w, r)
		})
	}

	// Management API
	manager := agents.NewManager(db, &activeCfg.Gitea)
	apiHandler := api.NewHandler(db, manager, activeCfg, jwtManager, cfgManager, func(newCfg *config.Config) {
		// Hot-reload LLM providers when config changes
		llmRegistry.Reload(&newCfg.LLM)
		llmRegistry.SetRateLimitBackoff(newCfg.Dispatcher.RateLimitBackoff, newCfg.LLM.RateLimitRetries)
		manager.ReloadGitea(&newCfg.Gitea)
		log.Printf("[INFO] LLM registry and Gitea client reloaded")
	})
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
