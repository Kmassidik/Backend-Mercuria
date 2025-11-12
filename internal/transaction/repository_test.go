package transaction

import (
	"context"
	"fmt"
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
		DBName:          "mercuria_transaction_test",
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

	// Create schema
	schema := `
	CREATE TABLE IF NOT EXISTS transactions (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		from_wallet_id VARCHAR(255) NOT NULL,
		to_wallet_id VARCHAR(255) NOT NULL,
		amount NUMERIC(20, 4) NOT NULL,
		currency VARCHAR(3) NOT NULL DEFAULT 'USD',
		type VARCHAR(20) NOT NULL,
		status VARCHAR(20) NOT NULL DEFAULT 'pending',
		description TEXT,
		idempotency_key VARCHAR(255) UNIQUE NOT NULL,
		scheduled_at TIMESTAMP WITH TIME ZONE,
		processed_at TIMESTAMP WITH TIME ZONE,
		failure_reason TEXT,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_transactions_from_wallet ON transactions(from_wallet_id);
	CREATE INDEX IF NOT EXISTS idx_transactions_to_wallet ON transactions(to_wallet_id);
	CREATE INDEX IF NOT EXISTS idx_transactions_status ON transactions(status);
	CREATE INDEX IF NOT EXISTS idx_transactions_idempotency ON transactions(idempotency_key);
	CREATE INDEX IF NOT EXISTS idx_transactions_scheduled ON transactions(scheduled_at) WHERE status = 'scheduled';

	CREATE TABLE IF NOT EXISTS batch_transactions (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		from_wallet_id VARCHAR(255) NOT NULL,
		total_amount NUMERIC(20, 4) NOT NULL,
		currency VARCHAR(3) NOT NULL DEFAULT 'USD',
		status VARCHAR(20) NOT NULL DEFAULT 'pending',
		idempotency_key VARCHAR(255) UNIQUE NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

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

	TRUNCATE transactions, batch_transactions, outbox_events CASCADE;
	`

	if _, err := database.Exec(schema); err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	repo := NewRepository(database, log)
	return repo, database
}

func cleanupTestDB(_ *testing.T, database *db.DB) {
	if database == nil {
		return
	}
	database.Exec("TRUNCATE transactions, batch_transactions, outbox_events CASCADE")
	database.Close()
}

// TEST: Create transaction
func TestCreateTransaction(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()

	txn := &Transaction{
		FromWalletID:   "wallet-123",
		ToWalletID:     "wallet-456",
		Amount:         "100.50",
		Currency:       "USD",
		Type:           TypeP2P,
		Status:         StatusPending,
		Description:    "Test transfer",
		IdempotencyKey: "txn-key-1",
	}

	created, err := repo.CreateTransaction(ctx, txn)
	if err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}

	if created.ID == "" {
		t.Error("Expected transaction ID to be set")
	}

	if created.Status != StatusPending {
		t.Errorf("Expected status pending, got %s", created.Status)
	}

	// Test duplicate idempotency key
	duplicate := &Transaction{
		FromWalletID:   "wallet-789",
		ToWalletID:     "wallet-999",
		Amount:         "50.00",
		Currency:       "USD",
		Type:           TypeP2P,
		Status:         StatusPending,
		IdempotencyKey: "txn-key-1", // Same key!
	}

	_, err = repo.CreateTransaction(ctx, duplicate)
	if err == nil {
		t.Error("Expected error for duplicate idempotency key")
	}
}

// TEST: Get transaction by ID
func TestGetTransaction(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()

	// Create transaction
	txn := &Transaction{
		FromWalletID:   "wallet-123",
		ToWalletID:     "wallet-456",
		Amount:         "75.25",
		Currency:       "USD",
		Type:           TypeP2P,
		Status:         StatusPending,
		IdempotencyKey: "txn-key-get",
	}

	created, err := repo.CreateTransaction(ctx, txn)
	if err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}

	// Get transaction
	found, err := repo.GetTransaction(ctx, created.ID)
	if err != nil {
		t.Fatalf("Failed to get transaction: %v", err)
	}

	if found.ID != created.ID {
		t.Errorf("Expected transaction ID %s, got %s", created.ID, found.ID)
	}

	if found.Amount != "75.2500" { // DB returns with full precision
		t.Errorf("Expected amount 75.2500, got %s", found.Amount)
	}
}

// TEST: Update transaction status
func TestUpdateTransactionStatus(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()

	// Create transaction
	txn := &Transaction{
		FromWalletID:   "wallet-123",
		ToWalletID:     "wallet-456",
		Amount:         "50.00",
		Currency:       "USD",
		Type:           TypeP2P,
		Status:         StatusPending,
		IdempotencyKey: "txn-key-status",
	}

	created, _ := repo.CreateTransaction(ctx, txn)

	// Update to completed
	err := repo.UpdateTransactionStatus(ctx, created.ID, StatusCompleted)
	if err != nil {
		t.Fatalf("Failed to update status: %v", err)
	}

	// Verify update
	updated, _ := repo.GetTransaction(ctx, created.ID)
	if updated.Status != StatusCompleted {
		t.Errorf("Expected status completed, got %s", updated.Status)
	}

	if updated.ProcessedAt == nil {
		t.Error("Expected processed_at to be set")
	}
}

// TEST: Mark transaction as failed
func TestMarkTransactionAsFailed(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()

	txn := &Transaction{
		FromWalletID:   "wallet-123",
		ToWalletID:     "wallet-456",
		Amount:         "100.00",
		Currency:       "USD",
		Type:           TypeP2P,
		Status:         StatusPending,
		IdempotencyKey: "txn-key-fail",
	}

	created, _ := repo.CreateTransaction(ctx, txn)

	// Mark as failed
	err := repo.MarkTransactionAsFailed(ctx, created.ID, "Insufficient balance")
	if err != nil {
		t.Fatalf("Failed to mark as failed: %v", err)
	}

	// Verify
	updated, _ := repo.GetTransaction(ctx, created.ID)
	if updated.Status != StatusFailed {
		t.Errorf("Expected status failed, got %s", updated.Status)
	}

	if updated.FailureReason != "Insufficient balance" {
		t.Errorf("Expected failure reason, got %s", updated.FailureReason)
	}
}

// TEST: Get scheduled transactions
func TestGetScheduledTransactions(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()
	now := time.Now()

	// Create scheduled transaction (due now)
	scheduledNow := &Transaction{
		FromWalletID:   "wallet-123",
		ToWalletID:     "wallet-456",
		Amount:         "50.00",
		Currency:       "USD",
		Type:           TypeScheduled,
		Status:         StatusScheduled,
		IdempotencyKey: "scheduled-now",
	}
	scheduledNow.ScheduledAt = &now
	repo.CreateTransaction(ctx, scheduledNow)

	// Create scheduled transaction (future)
	future := now.Add(24 * time.Hour)
	scheduledFuture := &Transaction{
		FromWalletID:   "wallet-789",
		ToWalletID:     "wallet-999",
		Amount:         "75.00",
		Currency:       "USD",
		Type:           TypeScheduled,
		Status:         StatusScheduled,
		IdempotencyKey: "scheduled-future",
	}
	scheduledFuture.ScheduledAt = &future
	repo.CreateTransaction(ctx, scheduledFuture)

	// Get due scheduled transactions
	due, err := repo.GetScheduledTransactions(ctx, 10)
	if err != nil {
		t.Fatalf("Failed to get scheduled transactions: %v", err)
	}

	// Should only return the one due now
	if len(due) != 1 {
		t.Errorf("Expected 1 due transaction, got %d", len(due))
	}

	if len(due) > 0 && due[0].IdempotencyKey != "scheduled-now" {
		t.Error("Expected to get the transaction scheduled for now")
	}
}

// TEST: List transactions by wallet
func TestListTransactionsByWallet(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()

	// Create multiple transactions
	for i := 0; i < 3; i++ {
		txn := &Transaction{
			FromWalletID:   "wallet-123",
			ToWalletID:     "wallet-456",
			Amount:         "50.00",
			Currency:       "USD",
			Type:           TypeP2P,
			Status:         StatusCompleted,
			IdempotencyKey: fmt.Sprintf("txn-list-%d", i),
		}
		repo.CreateTransaction(ctx, txn)
	}

	// List transactions
	transactions, err := repo.ListTransactionsByWallet(ctx, "wallet-123", 10, 0)
	if err != nil {
		t.Fatalf("Failed to list transactions: %v", err)
	}

	if len(transactions) != 3 {
		t.Errorf("Expected 3 transactions, got %d", len(transactions))
	}
}

// TEST: Create batch transaction
func TestCreateBatchTransaction(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()

	batch := &BatchTransaction{
		FromWalletID:   "wallet-123",
		TotalAmount:    "150.00",
		Currency:       "USD",
		Status:         StatusPending,
		IdempotencyKey: "batch-key-1",
		Transfers: []BatchTransferItem{
			{ToWalletID: "wallet-456", Amount: "50.00"},
			{ToWalletID: "wallet-789", Amount: "100.00"},
		},
	}

	created, err := repo.CreateBatchTransaction(ctx, batch)
	if err != nil {
		t.Fatalf("Failed to create batch: %v", err)
	}

	if created.ID == "" {
		t.Error("Expected batch ID to be set")
	}
}