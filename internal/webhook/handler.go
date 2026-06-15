package webhook

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"gitea-agent-gateway/internal/config"
)

// EventCallback is called when a webhook event is parsed and matched.
// It receives the parsed event for further processing (e.g., enqueue to dispatcher).
// Returns true if the event was successfully processed (task enqueued), false otherwise.
type EventCallback func(evt *WebhookEvent) bool

// Handler processes incoming Gitea webhook requests.
type Handler struct {
	cfg      *config.GiteaConfig
	dedup    *Deduplicator
	callback EventCallback
}

// NewHandler creates a new webhook Handler.
func NewHandler(cfg *config.GiteaConfig, db *sql.DB, callback EventCallback) *Handler {
	return &Handler{
		cfg:      cfg,
		dedup:    NewDeduplicator(db),
		callback: callback,
	}
}

// ServeHTTP handles the webhook POST request from Gitea.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read body
	body, err := ReadBody(r)
	if err != nil {
		log.Printf("[ERROR] Failed to read request body: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Verify signature
	if h.cfg.WebhookSecret != "" {
		if !VerifySignature(r, h.cfg.WebhookSecret, body) {
			log.Printf("[WARN] Invalid webhook signature from %s", r.RemoteAddr)
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}
	}

	// Get event type and delivery ID
	eventType := r.Header.Get("X-Gitea-Event")
	deliveryID := r.Header.Get("X-Gitea-Delivery")

	if eventType == "" {
		http.Error(w, "Missing X-Gitea-Event header", http.StatusBadRequest)
		return
	}

	// Dedup check
	if h.dedup.IsProcessed(deliveryID) {
		log.Printf("[INFO] Duplicate delivery %s, skipping", deliveryID)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"duplicate"}`)
		return
	}

	// Parse event
	evt, err := ParseEvent(eventType, deliveryID, body)
	if err != nil {
		log.Printf("[ERROR] Failed to parse event %s: %v", eventType, err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Dispatch event (async via callback)
	// MarkProcessed is called only after successful enqueue to avoid losing deliveries.
	if h.callback != nil {
		go func() {
			if h.callback(evt) {
				h.dedup.MarkProcessed(deliveryID)
			} else {
				log.Printf("[WARN] Event processing failed for delivery %s, not marking as processed", deliveryID)
			}
		}()
	}

	// Respond immediately
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}
