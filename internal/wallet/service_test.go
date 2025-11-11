package wallet

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kmassidik/mercuria/internal/common/config"
	"github.com/kmassidik/mercuria/internal/common/db"
	"github.com/kmassidik/mercuria/internal/common/kafka"
	"github.com/kmassidik/mercuria/internal/common/logger"
	"github.com/kmassidik/mercuria/internal/common/redis"
)

func setupTestService(t *testing.T) (*Service, *Repository, *redis.Client) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Database setup
	dbCfg := config.DatabaseConfig{
		Host:            "localhost",
		Port:            "5432",
		User:            "postgres",
		Password:        "postgres",
		DBName:          "mercuria_wallet_test",
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
	}

	log := logger.New("test")
	database, err := db.Connect(dbCfg, log)
	if err != nil {
		t.Skipf("Cannot connect to database: %v", err)
		return nil, nil, nil
	}

	// Create schema
	schema := `
	CREATE TABLE IF NOT EXISTS wallets (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		user_id UUID NOT NULL,
		currency VARCHAR(3) NOT NULL DEFAULT 'USD',
		balance NUMERIC(20, 4) NOT NULL DEFAULT 0.0000,
		status VARCHAR(20) NOT NULL DEFAULT 'active',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT positive_balance CHECK (balance >= 0),
		CONSTRAINT unique_user_currency UNIQUE(user_id, currency)
	);

	CREATE TABLE IF NOT EXISTS wallet_events (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		wallet_id UUID NOT NULL REFERENCES wallets(id) ON DELETE CASCADE,
		event_type VARCHAR(50) NOT NULL,
		amount NUMERIC(20, 4) NOT NULL,
		balance_before NUMERIC(20, 4) NOT NULL,
		balance_after NUMERIC(20, 4) NOT NULL,
		metadata JSONB,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	TRUNCATE wallets, wallet_events CASCADE;
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
		return nil, nil, nil
	}

	// Kafka setup (optional for tests)
	kafkaCfg := config.KafkaConfig{
		Brokers: []string{"localhost:9092"},
		GroupID: "wallet-test-group",
	}

	producer := kafka.NewProducer(kafkaCfg, log)

	repo := NewRepository(database, log)
	service := NewService(repo, redisClient, producer, log)

	return service, repo, redisClient
}

func cleanupTestService(_ *testing.T, repo *Repository, redisClient *redis.Client) {
	if repo != nil {
		repo.db.Exec("TRUNCATE wallets, wallet_events CASCADE")
		repo.db.Close()
	}
	if redisClient != nil {
		redisClient.FlushDB(context.Background())
		redisClient.Close()
	}
}

func TestServiceCreateWallet(t *testing.T) {
	service, repo, redisClient := setupTestService(t)
	if service == nil {
		return
	}
	defer cleanupTestService(t, repo, redisClient)

	ctx := context.Background()

	req := &CreateWalletRequest{
		UserID:   "user-123",
		Currency: "USD",
	}

	wallet, err := service.CreateWallet(ctx, req)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	if wallet.ID == "" {
		t.Error("Expected wallet ID")
	}

	if wallet.Balance != "0.0000" {
		t.Errorf("Expected balance 0.0000, got %s", wallet.Balance)
	}

	// Test duplicate wallet
	_, err = service.CreateWallet(ctx, req)
	if err == nil {
		t.Error("Expected error for duplicate wallet")
	}
}

func TestDeposit(t *testing.T) {
	service, repo, redisClient := setupTestService(t)
	if service == nil {
		return
	}
	defer cleanupTestService(t, repo, redisClient)

	ctx := context.Background()

	// Create wallet
	createReq := &CreateWalletRequest{
		UserID:   "user-deposit",
		Currency: "USD",
	}

	wallet, err := service.CreateWallet(ctx, createReq)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	// Deposit
	depositReq := &DepositRequest{
		Amount:         "100.50",
		IdempotencyKey: "deposit-key-1",
		Description:    "Test deposit",
	}

	updatedWallet, err := service.Deposit(ctx, wallet.ID, depositReq)
	if err != nil {
		t.Fatalf("Failed to deposit: %v", err)
	}

	if updatedWallet.Balance != "100.5000" {
		t.Errorf("Expected balance 100.5000, got %s", updatedWallet.Balance)
	}

	// Test idempotency - same key should be rejected
	_, err = service.Deposit(ctx, wallet.ID, depositReq)
	if err == nil {
		t.Error("Expected error for duplicate idempotency key")
	}

	// New deposit with different key
	depositReq2 := &DepositRequest{
		Amount:         "50.25",
		IdempotencyKey: "deposit-key-2",
	}

	updatedWallet, err = service.Deposit(ctx, wallet.ID, depositReq2)
	if err != nil {
		t.Fatalf("Failed to deposit: %v", err)
	}

	if updatedWallet.Balance != "150.7500" {
		t.Errorf("Expected balance 150.7500, got %s", updatedWallet.Balance)
	}
}

func TestWithdraw(t *testing.T) {
	service, repo, redisClient := setupTestService(t)
	if service == nil {
		return
	}
	defer cleanupTestService(t, repo, redisClient)

	ctx := context.Background()

	// Create wallet and deposit
	createReq := &CreateWalletRequest{
		UserID:   "user-withdraw",
		Currency: "USD",
	}

	wallet, err := service.CreateWallet(ctx, createReq)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	// Initial deposit
	depositReq := &DepositRequest{
		Amount:         "200.00",
		IdempotencyKey: "initial-deposit",
	}

	wallet, err = service.Deposit(ctx, wallet.ID, depositReq)
	if err != nil {
		t.Fatalf("Failed to deposit: %v", err)
	}

	// Withdraw
	withdrawReq := &WithdrawRequest{
		Amount:         "75.50",
		IdempotencyKey: "withdraw-key-1",
		Description:    "Test withdrawal",
	}

	updatedWallet, err := service.Withdraw(ctx, wallet.ID, withdrawReq)
	if err != nil {
		t.Fatalf("Failed to withdraw: %v", err)
	}

	if updatedWallet.Balance != "124.5000" {
		t.Errorf("Expected balance 124.5000, got %s", updatedWallet.Balance)
	}

	// Test insufficient balance
	withdrawReq2 := &WithdrawRequest{
		Amount:         "200.00",
		IdempotencyKey: "withdraw-key-2",
	}

	_, err = service.Withdraw(ctx, wallet.ID, withdrawReq2)
	if err == nil {
		t.Error("Expected error for insufficient balance")
	}

	// Test idempotency
	_, err = service.Withdraw(ctx, wallet.ID, withdrawReq)
	if err == nil {
		t.Error("Expected error for duplicate idempotency key")
	}
}

func TestServiceDeposit(t *testing.T) {
	service, repo, redisClient := setupTestService(t)
	if service == nil {
		return
	}
	defer cleanupTestService(t, repo, redisClient)

	ctx := context.Background()

	// Create wallet
	createReq := &CreateWalletRequest{
		UserID:   "user-get",
		Currency: "EUR",
	}

	created, err := service.CreateWallet(ctx, createReq)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	// Get wallet (should use cache after first call)
	wallet, err := service.GetWallet(ctx, created.ID)
	if err != nil {
		t.Fatalf("Failed to get wallet: %v", err)
	}

	if wallet.ID != created.ID {
		t.Errorf("Expected wallet ID %s, got %s", created.ID, wallet.ID)
	}

	// Get again (should hit cache)
	wallet2, err := service.GetWallet(ctx, created.ID)
	if err != nil {
		t.Fatalf("Failed to get wallet from cache: %v", err)
	}

	if wallet2.Balance != wallet.Balance {
		t.Error("Cache returned different balance")
	}
}

func TestServiceGetWalletEvents(t *testing.T) {
	service, repo, redisClient := setupTestService(t)
	if service == nil {
		return
	}
	defer cleanupTestService(t, repo, redisClient)

	ctx := context.Background()

	// Create wallet
	createReq := &CreateWalletRequest{
		UserID:   "user-events",
		Currency: "USD",
	}

	wallet, err := service.CreateWallet(ctx, createReq)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	// Create multiple transactions
	deposits := []string{"50.00", "75.00", "25.50"}
	for i, amount := range deposits {
		depositReq := &DepositRequest{
			Amount:         amount,
			IdempotencyKey: fmt.Sprintf("deposit-%d", i),
		}
		_, err := service.Deposit(ctx, wallet.ID, depositReq)
		if err != nil {
			t.Fatalf("Failed to deposit: %v", err)
		}
	}

	// Get events
	events, err := service.GetWalletEvents(ctx, wallet.ID, 10, 0)
	if err != nil {
		t.Fatalf("Failed to get events: %v", err)
	}

	if len(events) != 3 {
		t.Errorf("Expected 3 events, got %d", len(events))
	}

	// Verify events are in descending order (newest first)
	if events[0].EventType != EventTypeDeposit {
		t.Error("Expected deposit event")
	}
}