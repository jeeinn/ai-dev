package webhook

import (
	"database/sql"
	"log"
)

// Deduplicator checks and records webhook delivery IDs to prevent duplicate processing.
type Deduplicator struct {
	db *sql.DB
}

// NewDeduplicator creates a new Deduplicator backed by SQLite.
func NewDeduplicator(db *sql.DB) *Deduplicator {
	return &Deduplicator{db: db}
}

// IsProcessed returns true if the delivery ID has already been processed.
func (d *Deduplicator) IsProcessed(deliveryID string) bool {
	if deliveryID == "" {
		return false
	}
	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM processed_deliveries WHERE delivery_id = ?", deliveryID).Scan(&count)
	if err != nil {
		log.Printf("[WARN] Dedup check failed: %v", err)
		return false
	}
	return count > 0
}

// MarkProcessed records the delivery ID as processed.
func (d *Deduplicator) MarkProcessed(deliveryID string) {
	if deliveryID == "" {
		return
	}
	_, err := d.db.Exec("INSERT OR IGNORE INTO processed_deliveries (delivery_id) VALUES (?)", deliveryID)
	if err != nil {
		log.Printf("[WARN] Failed to mark delivery as processed: %v", err)
	}
}

// Cleanup removes delivery records older than the given number of days.
func (d *Deduplicator) Cleanup(days int) {
	_, err := d.db.Exec("DELETE FROM processed_deliveries WHERE created_at < datetime('now', ?)", "-"+string(rune('0'+days))+" days")
	if err != nil {
		log.Printf("[WARN] Dedup cleanup failed: %v", err)
	}
}
