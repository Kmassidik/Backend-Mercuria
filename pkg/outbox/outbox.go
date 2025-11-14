package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kmassidik/mercuria/internal/common/kafka"
	"github.com/kmassidik/mercuria/internal/common/logger"
)

// OutboxEvent represents an event waiting to be published to Kafka
// NOTE: This ensures exactly-once delivery - events are saved to DB first, then published

// type OutboxEvent struct {
// 	ID           string                 `json:"id"`
// 	AggregateID  string                 `json:"aggregate_id"`  // e.g., wallet_id, transaction_id
// 	EventType    string                 `json:"event_type"`    // e.g., "wallet.balance_updated"
// 	Topic        string                 `json:"topic"`         // Kafka topic name
// 	Payload      map[string]interface{} `json:"payload"`       // Event data
// 	Status       string                 `json:"status"`        // pending, published, failed
// 	Attempts     int                    `json:"attempts"`      // Retry counter
// 	LastError    string                 `json:"last_error"`    // Error message if failed
// 	CreatedAt    time.Time              `json:"created_at"`
// 	PublishedAt  *time.Time             `json:"published_at"`
// }

type OutboxEvent struct {
    ID           string                 `json:"id"`
    AggregateID  string                 `json:"aggregate_id"`
    EventType    string                 `json:"event_type"`
    Topic        string                 `json:"topic"`
    Payload      map[string]interface{} `json:"payload"`
    Status       string                 `json:"status"`
    Attempts     int                    `json:"attempts"`
    LastError    sql.NullString         `json:"last_error"`    // <-- FIX: Changed to sql.NullString
    CreatedAt    time.Time              `json:"created_at"`
    PublishedAt  sql.NullTime           `json:"published_at"`  // <-- GOOD PRACTICE: Changed from *time.Time
}

const (
	StatusPending   = "pending"
	StatusPublished = "published"
	StatusFailed    = "failed"
)

type Repository struct {
	db     *sql.DB
	logger *logger.Logger
}

func NewRepository(db *sql.DB, log *logger.Logger) *Repository {
	return &Repository{
		db:     db,
		logger: log,
	}
}

// SaveEvent saves an event to the outbox table within a transaction
// NOTE: This is called INSIDE your business transaction to ensure atomicity
// Example: When depositing to wallet, save deposit event in same transaction
func (r *Repository) SaveEvent(ctx context.Context, tx *sql.Tx, event *OutboxEvent) error {
	payloadJSON, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	query := `
		INSERT INTO outbox_events (aggregate_id, event_type, topic, payload, status, attempts)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`

	event.Status = StatusPending
	event.Attempts = 0

	err = tx.QueryRowContext(
		ctx,
		query,
		event.AggregateID,
		event.EventType,
		event.Topic,
		payloadJSON,
		event.Status,
		event.Attempts,
	).Scan(&event.ID, &event.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to save outbox event: %w", err)
	}

	r.logger.Debugf("Outbox event saved: %s for aggregate %s", event.EventType, event.AggregateID)
	return nil
}

// GetPendingEvents retrieves events that need to be published
// NOTE: This is called by the background worker to publish events to Kafka
func (r *Repository) GetPendingEvents(ctx context.Context, limit int) ([]OutboxEvent, error) {
	query := `
        SELECT id, aggregate_id, event_type, topic, payload, status, attempts, last_error, created_at, published_at
        FROM outbox_events
        WHERE status = $1 AND attempts < 5
        ORDER BY created_at ASC
        LIMIT $2
    `

	rows, err := r.db.QueryContext(ctx, query, StatusPending, limit)
    if err != nil {
        return nil, fmt.Errorf("failed to get pending events: %w", err)
    }
    defer rows.Close()

	var events []OutboxEvent
	for rows.Next() {
		var event OutboxEvent
		var payloadJSON []byte
		// Variables for nullable fields
        var lastError sql.NullString 
        var publishedAt sql.NullTime
		
		err := rows.Scan(
            &event.ID,
            &event.AggregateID,
            &event.EventType,
            &event.Topic,
            &payloadJSON,
            &event.Status,
            &event.Attempts,
            &lastError,          // Scan into sql.NullString
            &event.CreatedAt,
            &publishedAt,        // Scan into sql.NullTime
        )

		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}
		
		// Assign nullable variables back to the struct fields
        event.LastError = lastError
        event.PublishedAt = publishedAt
		
		// Unmarshal payload
        if err := json.Unmarshal(payloadJSON, &event.Payload); err != nil {
            r.logger.Warnf("Failed to unmarshal payload for event %s: %v", event.ID, err)
            continue
        }

        events = append(events, event)
	}

	return events, nil
}

// MarkAsPublished marks an event as successfully published
// NOTE: Called after Kafka confirms the event was published
func (r *Repository) MarkAsPublished(ctx context.Context, eventID string) error {
	query := `
		UPDATE outbox_events
		SET status = $1, published_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`

	result, err := r.db.ExecContext(ctx, query, StatusPublished, eventID)
	if err != nil {
		return fmt.Errorf("failed to mark event as published: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("event not found: %s", eventID)
	}

	r.logger.Debugf("Event marked as published: %s", eventID)
	return nil
}

// MarkAsFailed marks an event as failed after max retries
// NOTE: Called when event publishing fails repeatedly
func (r *Repository) MarkAsFailed(ctx context.Context, eventID string, errorMsg string) error {
	query := `
		UPDATE outbox_events
		SET status = $1, attempts = attempts + 1, last_error = $2
		WHERE id = $3
	`

	_, err := r.db.ExecContext(ctx, query, StatusFailed, errorMsg, eventID)
	if err != nil {
		return fmt.Errorf("failed to mark event as failed: %w", err)
	}

	r.logger.Warnf("Event marked as failed: %s - %s", eventID, errorMsg)
	return nil
}

// IncrementAttempt increments the retry attempt counter
// NOTE: Called when publishing fails but we want to retry
func (r *Repository) IncrementAttempt(ctx context.Context, eventID string, errorMsg string) error {
	query := `
		UPDATE outbox_events
		SET attempts = attempts + 1, last_error = $1
		WHERE id = $2
	`

	_, err := r.db.ExecContext(ctx, query, errorMsg, eventID)
	if err != nil {
		return fmt.Errorf("failed to increment attempt: %w", err)
	}

	return nil
}

// Publisher is responsible for publishing outbox events to Kafka
// NOTE: This runs as a background worker, polling the outbox table
type Publisher struct {
	repo     *Repository
	producer *kafka.Producer
	logger   *logger.Logger
	interval time.Duration // How often to poll for new events
}

func NewPublisher(repo *Repository, producer *kafka.Producer, log *logger.Logger, interval time.Duration) *Publisher {
	return &Publisher{
		repo:     repo,
		producer: producer,
		logger:   log,
		interval: interval,
	}
}

// Start begins the background worker that publishes events
// NOTE: Call this in your main.go after service initialization
// Example: go publisher.Start(ctx)
func (p *Publisher) Start(ctx context.Context) {
	p.logger.Info("Outbox publisher started")
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("Outbox publisher stopped")
			return
		case <-ticker.C:
			if err := p.publishPendingEvents(ctx); err != nil {
				p.logger.Errorf("Failed to publish pending events: %v", err)
			}
		}
	}
}

// publishPendingEvents fetches and publishes pending events
// NOTE: This is the core outbox processing logic
func (p *Publisher) publishPendingEvents(ctx context.Context) error {
	// Get pending events (limit to 100 per batch)
	events, err := p.repo.GetPendingEvents(ctx, 100)
	if err != nil {
		return fmt.Errorf("failed to get pending events: %w", err)
	}

	if len(events) == 0 {
		return nil
	}

	p.logger.Infof("Publishing %d pending events", len(events))

	for _, event := range events {
		// Publish to Kafka
		err := p.producer.PublishEvent(ctx, event.Topic, event.AggregateID, event.Payload)
		if err != nil {
			// Increment attempt counter
			p.logger.Errorf("Failed to publish event %s: %v", event.ID, err)
			
			if event.Attempts >= 4 { // Max 5 attempts (0-4)
				p.repo.MarkAsFailed(ctx, event.ID, err.Error())
			} else {
				p.repo.IncrementAttempt(ctx, event.ID, err.Error())
			}
			continue
		}

		// Mark as published
		if err := p.repo.MarkAsPublished(ctx, event.ID); err != nil {
			p.logger.Errorf("Failed to mark event as published: %v", err)
		}
	}

	return nil
}