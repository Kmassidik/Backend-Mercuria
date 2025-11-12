package outbox

import (
	"context"
	"testing"
	"time"

	"github.com/kmassidik/mercuria/internal/common/config"
	"github.com/kmassidik/mercuria/internal/common/db"
	"github.com/kmassidik/mercuria/internal/common/logger"
)

func setupTestDB(t *testing.T) (*Repository, *db.DB) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	cfg := config.DatabaseConfig{
		Host:            "localhost",
		Port:            "5432",
		User:            "postgres",
		Password:        "postgres",
		DBName:          "mercuria_outbox_test",
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
	}

	log := logger.New("test")
	database, err := db.Connect(cfg, log)
	if err != nil {
		t.Skipf("Cannot connect to database: %v", err)
		return nil, nil
	}

	// Create outbox table
	schema := `
	CREATE TABLE IF NOT EXISTS outbox_events (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		aggregate_id VARCHAR(255) NOT NULL,
		event_type VARCHAR(100) NOT NULL,
		topic VARCHAR(100) NOT NULL,
		payload JSONB NOT NULL,
		status VARCHAR(20) NOT NULL DEFAULT 'pending',
		attempts INT NOT NULL DEFAULT 0,
		last_error TEXT,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		published_at TIMESTAMP WITH TIME ZONE
	);

	CREATE INDEX IF NOT EXISTS idx_outbox_status ON outbox_events(status, created_at);
	CREATE INDEX IF NOT EXISTS idx_outbox_aggregate ON outbox_events(aggregate_id);

	TRUNCATE outbox_events CASCADE;
	`

	if _, err := database.Exec(schema); err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	repo := NewRepository(database.DB, log)
	return repo, database
}

func cleanupTestDB(_ *testing.T, database *db.DB) {
	if database == nil {
		return
	}
	database.Exec("TRUNCATE outbox_events CASCADE")
	database.Close()
}

// TEST: Save outbox event within a transaction
func TestSaveEvent(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()

	// Start transaction
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Create outbox event
	event := &OutboxEvent{
		AggregateID: "wallet-123",
		EventType:   "wallet.balance_updated",
		Topic:       "wallet.balance_updated",
		Payload: map[string]interface{}{
			"wallet_id": "wallet-123",
			"amount":    "100.50",
		},
	}

	// Save event
	err = repo.SaveEvent(ctx, tx, event)
	if err != nil {
		t.Fatalf("Failed to save event: %v", err)
	}

	// Verify event ID was set
	if event.ID == "" {
		t.Error("Expected event ID to be set")
	}

	// Verify status is pending
	if event.Status != StatusPending {
		t.Errorf("Expected status pending, got %s", event.Status)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}
}

// TEST: Get pending events for publishing
func TestGetPendingEvents(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()

	// Create multiple outbox events
	for i := 0; i < 3; i++ {
		tx, _ := database.BeginTx(ctx, nil)
		event := &OutboxEvent{
			AggregateID: "wallet-123",
			EventType:   "wallet.deposit",
			Topic:       "wallet.balance_updated",
			Payload: map[string]interface{}{
				"amount": "50.00",
			},
		}
		repo.SaveEvent(ctx, tx, event)
		tx.Commit()
	}

	// Get pending events
	events, err := repo.GetPendingEvents(ctx, 10)
	if err != nil {
		t.Fatalf("Failed to get pending events: %v", err)
	}

	if len(events) != 3 {
		t.Errorf("Expected 3 events, got %d", len(events))
	}

	// Verify events are ordered by created_at
	if len(events) >= 2 {
		if events[0].CreatedAt.After(events[1].CreatedAt) {
			t.Error("Events should be ordered by created_at ASC")
		}
	}
}

// TEST: Mark event as published
func TestMarkAsPublished(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()

	// Create event
	tx, _ := database.BeginTx(ctx, nil)
	event := &OutboxEvent{
		AggregateID: "wallet-456",
		EventType:   "wallet.created",
		Topic:       "wallet.created",
		Payload:     map[string]interface{}{"wallet_id": "wallet-456"},
	}
	repo.SaveEvent(ctx, tx, event)
	tx.Commit()

	// Mark as published
	err := repo.MarkAsPublished(ctx, event.ID)
	if err != nil {
		t.Fatalf("Failed to mark as published: %v", err)
	}

	// Verify event is not in pending list
	events, _ := repo.GetPendingEvents(ctx, 10)
	for _, e := range events {
		if e.ID == event.ID {
			t.Error("Event should not be in pending list after marking as published")
		}
	}
}

// TEST: Mark event as failed after max retries
func TestMarkAsFailed(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()

	// Create event
	tx, _ := database.BeginTx(ctx, nil)
	event := &OutboxEvent{
		AggregateID: "wallet-789",
		EventType:   "wallet.deposit",
		Topic:       "wallet.balance_updated",
		Payload:     map[string]interface{}{"amount": "100.00"},
	}
	repo.SaveEvent(ctx, tx, event)
	tx.Commit()

	// Mark as failed
	err := repo.MarkAsFailed(ctx, event.ID, "Kafka broker unavailable")
	if err != nil {
		t.Fatalf("Failed to mark as failed: %v", err)
	}

	// Verify event is not in pending list
	events, _ := repo.GetPendingEvents(ctx, 10)
	for _, e := range events {
		if e.ID == event.ID {
			t.Error("Failed event should not be in pending list")
		}
	}
}

// TEST: Increment attempt counter
func TestIncrementAttempt(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()

	// Create event
	tx, _ := database.BeginTx(ctx, nil)
	event := &OutboxEvent{
		AggregateID: "wallet-999",
		EventType:   "wallet.withdrawal",
		Topic:       "wallet.balance_updated",
		Payload:     map[string]interface{}{"amount": "50.00"},
	}
	repo.SaveEvent(ctx, tx, event)
	tx.Commit()

	// Increment attempt multiple times
	for i := 0; i < 3; i++ {
		err := repo.IncrementAttempt(ctx, event.ID, "Temporary failure")
		if err != nil {
			t.Fatalf("Failed to increment attempt: %v", err)
		}
	}

	// Verify attempts were incremented
	events, _ := repo.GetPendingEvents(ctx, 10)
	found := false
	for _, e := range events {
		if e.ID == event.ID {
			found = true
			if e.Attempts != 3 {
				t.Errorf("Expected 3 attempts, got %d", e.Attempts)
			}
		}
	}

	if !found {
		t.Error("Event should still be in pending list")
	}
}

// TEST: Events with max attempts are excluded from pending
func TestMaxAttemptsExclusion(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()

	// Create event and set attempts to 5 (max)
	tx, _ := database.BeginTx(ctx, nil)
	event := &OutboxEvent{
		AggregateID: "wallet-max",
		EventType:   "wallet.deposit",
		Topic:       "wallet.balance_updated",
		Payload:     map[string]interface{}{"amount": "10.00"},
	}
	repo.SaveEvent(ctx, tx, event)
	tx.Commit()

	// Increment to max attempts
	for i := 0; i < 5; i++ {
		repo.IncrementAttempt(ctx, event.ID, "Retry failed")
	}

	// Verify event is excluded from pending
	events, _ := repo.GetPendingEvents(ctx, 10)
	for _, e := range events {
		if e.ID == event.ID {
			t.Error("Event with max attempts should not be in pending list")
		}
	}
}