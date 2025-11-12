package transaction

import (
	"context"
	"testing"
	"time"

	"github.com/kmassidik/mercuria/internal/common/config"
	"github.com/kmassidik/mercuria/internal/common/db"
	"github.com/kmassidik/mercuria/internal/common/kafka"
	"github.com/kmassidik/mercuria/internal/common/logger"
	"github.com/kmassidik/mercuria/internal/common/redis"
	"github.com/kmassidik/mercuria/pkg/outbox"
)

// MockWalletRepository mocks wallet operations
// NOTE: Transaction service needs to interact with wallets
type MockWalletRepository struct {
	GetWalletFunc       func(ctx context.Context, id string) (*MockWallet, error)
	UpdateBalanceFunc   func(ctx context.Context, id, newBalance string) error
	GetWalletForUpdateFunc func(ctx context.Context, id string) (*MockWallet, error)
}

type MockWallet struct {
	ID       string
	UserID   string
	Currency string
	Balance  string
	Status   string
}

func setupTestService(t *testing.T) (*Service, *Repository, *redis.Client, *outbox.Repository) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Database setup
	dbCfg := config.DatabaseConfig{
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
	database, err := db.Connect(dbCfg, log)
	if err != nil {
		t.Skipf("Cannot connect to database: %v", err)
		return nil, nil, nil, nil
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

	CREATE TABLE IF NOT EXISTS wallets (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		user_id UUID NOT NULL,
		currency VARCHAR(3) NOT NULL DEFAULT 'USD',
		balance NUMERIC(20, 4) NOT NULL DEFAULT 0.0000,
		status VARCHAR(20) NOT NULL DEFAULT 'active',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	TRUNCATE transactions, batch_transactions, outbox_events, wallets CASCADE;
	`

	if _, err := database.Exec(schema); err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	// Redis setup
	redisCfg := config.RedisConfig{
		Host:     "localhost",
		Port:     "6379",
		Password: "",
		DB:       0,
	}

	redisClient, err := redis.Connect(redisCfg, log)
	if err != nil {
		t.Skipf("Cannot connect to Redis: %v", err)
		return nil, nil, nil, nil
	}

	// Kafka setup
	kafkaCfg := config.KafkaConfig{
		Brokers: []string{"localhost:9092"},
		GroupID: "transaction-test-group",
	}
	producer := kafka.NewProducer(kafkaCfg, log)

	// Create repositories
	txnRepo := NewRepository(database, log)
	outboxRepo := outbox.NewRepository(database.DB, log)

	// Create service
	service := NewService(txnRepo, outboxRepo, redisClient, producer, database, log)

	return service, txnRepo, redisClient, outboxRepo
}

func cleanupTestService(_ *testing.T, database *db.DB, redisClient *redis.Client) {
	if database != nil {
		database.Exec("TRUNCATE transactions, batch_transactions, outbox_events, wallets CASCADE")
		database.Close()
	}
	if redisClient != nil {
		redisClient.FlushDB(context.Background())
		redisClient.Close()
	}
}

// Helper: Create test wallet in database
func createTestWallet(t *testing.T, db *db.DB, walletID, userID, balance string) {
	query := `INSERT INTO wallets (id, user_id, balance, currency, status) VALUES ($1, $2, $3, 'USD', 'active')`
	_, err := db.Exec(query, walletID, userID, balance)
	if err != nil {
		t.Fatalf("Failed to create test wallet: %v", err)
	}
}

// Helper: Get wallet balance from database
func getWalletBalance(t *testing.T, db *db.DB, walletID string) string {
	var balance string
	query := `SELECT balance FROM wallets WHERE id = $1`
	err := db.QueryRow(query, walletID).Scan(&balance)
	if err != nil {
		t.Fatalf("Failed to get wallet balance: %v", err)
	}
	return balance
}

// TEST: Create simple P2P transfer
func TestCreateP2PTransfer(t *testing.T) {
	service, txnRepo, redisClient, _ := setupTestService(t)
	if service == nil {
		return
	}
	defer cleanupTestService(t, txnRepo.db, redisClient)

	ctx := context.Background()

	// Create test wallets
	createTestWallet(t, txnRepo.db, "wallet-from", "user-123", "500.0000")
	createTestWallet(t, txnRepo.db, "wallet-to", "user-456", "100.0000")

	// Create transfer request
	req := &CreateTransactionRequest{
		FromWalletID:   "wallet-from",
		ToWalletID:     "wallet-to",
		Amount:         "50.00",
		Description:    "Test transfer",
		IdempotencyKey: "txn-key-1",
	}

	// Execute transfer
	txn, err := service.CreateP2PTransfer(ctx, req)
	if err != nil {
		t.Fatalf("Failed to create transfer: %v", err)
	}

	// Verify transaction created
	if txn.ID == "" {
		t.Error("Expected transaction ID")
	}

	if txn.Status != StatusCompleted {
		t.Errorf("Expected status completed, got %s", txn.Status)
	}

	// Verify balances updated
	fromBalance := getWalletBalance(t, txnRepo.db, "wallet-from")
	if fromBalance != "450.0000" {
		t.Errorf("Expected from balance 450.0000, got %s", fromBalance)
	}

	toBalance := getWalletBalance(t, txnRepo.db, "wallet-to")
	if toBalance != "150.0000" {
		t.Errorf("Expected to balance 150.0000, got %s", toBalance)
	}
}

// TEST: Idempotency enforcement
func TestTransferIdempotency(t *testing.T) {
	service, txnRepo, redisClient, _ := setupTestService(t)
	if service == nil {
		return
	}
	defer cleanupTestService(t, txnRepo.db, redisClient)

	ctx := context.Background()

	createTestWallet(t, txnRepo.db, "wallet-from", "user-123", "500.0000")
	createTestWallet(t, txnRepo.db, "wallet-to", "user-456", "100.0000")

	req := &CreateTransactionRequest{
		FromWalletID:   "wallet-from",
		ToWalletID:     "wallet-to",
		Amount:         "50.00",
		IdempotencyKey: "duplicate-key",
	}

	// First request should succeed
	_, err := service.CreateP2PTransfer(ctx, req)
	if err != nil {
		t.Fatalf("First transfer failed: %v", err)
	}

	// Second request with same key should fail
	_, err = service.CreateP2PTransfer(ctx, req)
	if err == nil {
		t.Error("Expected error for duplicate idempotency key")
	}

	// Verify balance only changed once
	fromBalance := getWalletBalance(t, txnRepo.db, "wallet-from")
	if fromBalance != "450.0000" {
		t.Errorf("Balance should only be deducted once, got %s", fromBalance)
	}
}

// TEST: Insufficient balance
func TestInsufficientBalance(t *testing.T) {
	service, txnRepo, redisClient, _ := setupTestService(t)
	if service == nil {
		return
	}
	defer cleanupTestService(t, txnRepo.db, redisClient)

	ctx := context.Background()

	createTestWallet(t, txnRepo.db, "wallet-from", "user-123", "10.0000")
	createTestWallet(t, txnRepo.db, "wallet-to", "user-456", "100.0000")

	req := &CreateTransactionRequest{
		FromWalletID:   "wallet-from",
		ToWalletID:     "wallet-to",
		Amount:         "50.00",
		IdempotencyKey: "insufficient-key",
	}

	_, err := service.CreateP2PTransfer(ctx, req)
	if err == nil {
		t.Error("Expected insufficient balance error")
	}

	// Verify balances unchanged
	fromBalance := getWalletBalance(t, txnRepo.db, "wallet-from")
	if fromBalance != "10.0000" {
		t.Error("Source balance should not change on failure")
	}
}

// TEST: Create batch transfer
func TestCreateBatchTransfer(t *testing.T) {
	service, txnRepo, redisClient, _ := setupTestService(t)
	if service == nil {
		return
	}
	defer cleanupTestService(t, txnRepo.db, redisClient)

	ctx := context.Background()

	createTestWallet(t, txnRepo.db, "wallet-from", "user-123", "500.0000")
	createTestWallet(t, txnRepo.db, "wallet-to-1", "user-456", "100.0000")
	createTestWallet(t, txnRepo.db, "wallet-to-2", "user-789", "50.0000")

	req := &CreateBatchTransactionRequest{
		FromWalletID: "wallet-from",
		Transfers: []BatchTransferItem{
			{ToWalletID: "wallet-to-1", Amount: "50.00", Description: "Payment 1"},
			{ToWalletID: "wallet-to-2", Amount: "75.00", Description: "Payment 2"},
		},
		IdempotencyKey: "batch-key-1",
	}

	batch, txns, err := service.CreateBatchTransfer(ctx, req)
	if err != nil {
		t.Fatalf("Failed to create batch: %v", err)
	}

	if batch.ID == "" {
		t.Error("Expected batch ID")
	}

	if len(txns) != 2 {
		t.Errorf("Expected 2 transactions, got %d", len(txns))
	}

	// Verify total deducted
	fromBalance := getWalletBalance(t, txnRepo.db, "wallet-from")
	if fromBalance != "375.0000" { // 500 - 50 - 75
		t.Errorf("Expected from balance 375.0000, got %s", fromBalance)
	}

	// Verify recipients credited
	to1Balance := getWalletBalance(t, txnRepo.db, "wallet-to-1")
	if to1Balance != "150.0000" {
		t.Errorf("Expected to-1 balance 150.0000, got %s", to1Balance)
	}

	to2Balance := getWalletBalance(t, txnRepo.db, "wallet-to-2")
	if to2Balance != "125.0000" {
		t.Errorf("Expected to-2 balance 125.0000, got %s", to2Balance)
	}
}

// TEST: Batch atomicity (all or nothing)
func TestBatchAtomicity(t *testing.T) {
	service, txnRepo, redisClient, _ := setupTestService(t)
	if service == nil {
		return
	}
	defer cleanupTestService(t, txnRepo.db, redisClient)

	ctx := context.Background()

	createTestWallet(t, txnRepo.db, "wallet-from", "user-123", "100.0000")
	createTestWallet(t, txnRepo.db, "wallet-to-1", "user-456", "0.0000")
	// Note: wallet-to-2 doesn't exist - should cause batch to fail

	req := &CreateBatchTransactionRequest{
		FromWalletID: "wallet-from",
		Transfers: []BatchTransferItem{
			{ToWalletID: "wallet-to-1", Amount: "50.00"},
			{ToWalletID: "wallet-to-2", Amount: "25.00"}, // Non-existent wallet
		},
		IdempotencyKey: "batch-atomic-key",
	}

	_, _, err := service.CreateBatchTransfer(ctx, req)
	if err == nil {
		t.Error("Expected error for non-existent recipient")
	}

	// Verify NO balances changed (rollback)
	fromBalance := getWalletBalance(t, txnRepo.db, "wallet-from")
	if fromBalance != "100.0000" {
		t.Error("Source balance should not change on batch failure")
	}

	to1Balance := getWalletBalance(t, txnRepo.db, "wallet-to-1")
	if to1Balance != "0.0000" {
		t.Error("First recipient should not receive funds on batch failure")
	}
}

// TEST: Create scheduled transfer
func TestCreateScheduledTransfer(t *testing.T) {
	service, txnRepo, redisClient, _ := setupTestService(t)
	if service == nil {
		return
	}
	defer cleanupTestService(t, txnRepo.db, redisClient)

	ctx := context.Background()

	createTestWallet(t, txnRepo.db, "wallet-from", "user-123", "500.0000")
	createTestWallet(t, txnRepo.db, "wallet-to", "user-456", "100.0000")

	scheduledTime := time.Now().Add(1 * time.Hour)

	req := &CreateScheduledTransactionRequest{
		FromWalletID:   "wallet-from",
		ToWalletID:     "wallet-to",
		Amount:         "50.00",
		Description:    "Future payment",
		ScheduledAt:    scheduledTime,
		IdempotencyKey: "scheduled-key-1",
	}

	txn, err := service.CreateScheduledTransfer(ctx, req)
	if err != nil {
		t.Fatalf("Failed to create scheduled transfer: %v", err)
	}

	if txn.Status != StatusScheduled {
		t.Errorf("Expected status scheduled, got %s", txn.Status)
	}

	// Verify balances NOT changed yet
	fromBalance := getWalletBalance(t, txnRepo.db, "wallet-from")
	if fromBalance != "500.0000" {
		t.Error("Balance should not change until scheduled time")
	}
}

// TEST: Process scheduled transfers (worker)
func TestProcessScheduledTransfers(t *testing.T) {
	service, txnRepo, redisClient, _ := setupTestService(t)
	if service == nil {
		return
	}
	defer cleanupTestService(t, txnRepo.db, redisClient)

	ctx := context.Background()

	createTestWallet(t, txnRepo.db, "wallet-from", "user-123", "500.0000")
	createTestWallet(t, txnRepo.db, "wallet-to", "user-456", "100.0000")

	// Create scheduled transfer due now
	pastTime := time.Now().Add(-1 * time.Minute)
	req := &CreateScheduledTransactionRequest{
		FromWalletID:   "wallet-from",
		ToWalletID:     "wallet-to",
		Amount:         "50.00",
		ScheduledAt:    pastTime,
		IdempotencyKey: "process-scheduled-key",
	}

	txn, _ := service.CreateScheduledTransfer(ctx, req)

	// Process scheduled transfers
	processed, err := service.ProcessScheduledTransfers(ctx)
	if err != nil {
		t.Fatalf("Failed to process scheduled transfers: %v", err)
	}

	if processed != 1 {
		t.Errorf("Expected 1 processed, got %d", processed)
	}

	// Verify transaction completed
	updated, _ := txnRepo.GetTransaction(ctx, txn.ID)
	if updated.Status != StatusCompleted {
		t.Errorf("Expected status completed, got %s", updated.Status)
	}

	// Verify balances changed
	fromBalance := getWalletBalance(t, txnRepo.db, "wallet-from")
	if fromBalance != "450.0000" {
		t.Errorf("Expected from balance 450.0000, got %s", fromBalance)
	}
}

// TEST: Currency mismatch
func TestCurrencyMismatch(t *testing.T) {
	service, txnRepo, redisClient, _ := setupTestService(t)
	if service == nil {
		return
	}
	defer cleanupTestService(t, txnRepo.db, redisClient)

	ctx := context.Background()

	// Create wallets with different currencies
	query := `INSERT INTO wallets (id, user_id, balance, currency, status) VALUES ($1, $2, $3, $4, 'active')`
	txnRepo.db.Exec(query, "wallet-usd", "user-123", "500.0000", "USD")
	txnRepo.db.Exec(query, "wallet-eur", "user-456", "100.0000", "EUR")

	req := &CreateTransactionRequest{
		FromWalletID:   "wallet-usd",
		ToWalletID:     "wallet-eur",
		Amount:         "50.00",
		IdempotencyKey: "currency-mismatch-key",
	}

	_, err := service.CreateP2PTransfer(ctx, req)
	if err == nil {
		t.Error("Expected currency mismatch error")
	}
}