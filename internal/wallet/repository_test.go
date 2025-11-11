package wallet

import (
	"context"
	"testing"
	"time"

	"github.com/kmassidik/mercuria/internal/common/config"
	"github.com/kmassidik/mercuria/internal/common/db"
	"github.com/kmassidik/mercuria/internal/common/logger"
)

func setupTestDB(t *testing.T) *Repository {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	cfg := config.DatabaseConfig{
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
	database, err := db.Connect(cfg, log)
	if err != nil {
		t.Skipf("Cannot connect to database: %v", err)
		return nil
	}

	// Create tables
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

	return NewRepository(database, log)
}

func cleanupTestDB(_ *testing.T, repo *Repository) {
	if repo == nil {
		return
	}
	repo.db.Exec("TRUNCATE wallets, wallet_events CASCADE")
	repo.db.Close()
}

func TestCreateWallet(t *testing.T) {
	repo := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, repo)

	ctx := context.Background()

	wallet := &Wallet{
		UserID:   "user-123",
		Currency: "USD",
		Balance:  "0.0000",
		Status:   StatusActive,
	}

	created, err := repo.CreateWallet(ctx, wallet)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	if created.ID == "" {
		t.Error("Expected wallet ID to be set")
	}

	if created.Balance != "0.0000" {
		t.Errorf("Expected initial balance 0.0000, got %s", created.Balance)
	}

	// Test duplicate wallet creation should fail
	_, err = repo.CreateWallet(ctx, wallet)
	if err == nil {
		t.Error("Expected error for duplicate wallet")
	}
}

func TestGetWallet(t *testing.T) {
	repo := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, repo)

	ctx := context.Background()

	// Create wallet
	wallet := &Wallet{
		UserID:   "user-456",
		Currency: "EUR",
		Balance:  "0.0000",
		Status:   StatusActive,
	}

	created, err := repo.CreateWallet(ctx, wallet)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	// Get wallet
	found, err := repo.GetWallet(ctx, created.ID)
	if err != nil {
		t.Fatalf("Failed to get wallet: %v", err)
	}

	if found.ID != created.ID {
		t.Errorf("Expected wallet ID %s, got %s", created.ID, found.ID)
	}

	if found.Currency != "EUR" {
		t.Errorf("Expected currency EUR, got %s", found.Currency)
	}
}

func TestGetWalletByUserAndCurrency(t *testing.T) {
	repo := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, repo)

	ctx := context.Background()

	wallet := &Wallet{
		UserID:   "user-789",
		Currency: "GBP",
		Balance:  "0.0000",
		Status:   StatusActive,
	}

	_, err := repo.CreateWallet(ctx, wallet)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	// Get by user and currency
	found, err := repo.GetWalletByUserAndCurrency(ctx, "user-789", "GBP")
	if err != nil {
		t.Fatalf("Failed to get wallet: %v", err)
	}

	if found.UserID != "user-789" {
		t.Errorf("Expected user ID user-789, got %s", found.UserID)
	}

	// Non-existent wallet
	_, err = repo.GetWalletByUserAndCurrency(ctx, "user-789", "JPY")
	if err == nil {
		t.Error("Expected error for non-existent wallet")
	}
}

func TestUpdateBalance(t *testing.T) {
	repo := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, repo)

	ctx := context.Background()

	wallet := &Wallet{
		UserID:   "user-update",
		Currency: "USD",
		Balance:  "100.0000",
		Status:   StatusActive,
	}

	created, err := repo.CreateWallet(ctx, wallet)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	// Update balance
	err = repo.UpdateBalance(ctx, created.ID, "250.5000")
	if err != nil {
		t.Fatalf("Failed to update balance: %v", err)
	}

	// Verify balance updated
	updated, err := repo.GetWallet(ctx, created.ID)
	if err != nil {
		t.Fatalf("Failed to get updated wallet: %v", err)
	}

	if updated.Balance != "250.5000" {
		t.Errorf("Expected balance 250.5000, got %s", updated.Balance)
	}
}

func TestCreateWalletEvent(t *testing.T) {
	repo := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, repo)

	ctx := context.Background()

	wallet := &Wallet{
		UserID:   "user-event",
		Currency: "USD",
		Balance:  "0.0000",
		Status:   StatusActive,
	}

	created, err := repo.CreateWallet(ctx, wallet)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	// Create event
	event := &WalletEvent{
		WalletID:      created.ID,
		EventType:     EventTypeDeposit,
		Amount:        "100.0000",
		BalanceBefore: "0.0000",
		BalanceAfter:  "100.0000",
		Metadata: map[string]interface{}{
			"description": "Initial deposit",
		},
	}

	createdEvent, err := repo.CreateWalletEvent(ctx, event)
	if err != nil {
		t.Fatalf("Failed to create event: %v", err)
	}

	if createdEvent.ID == "" {
		t.Error("Expected event ID to be set")
	}

	if createdEvent.EventType != EventTypeDeposit {
		t.Errorf("Expected event type %s, got %s", EventTypeDeposit, createdEvent.EventType)
	}
}

func TestGetWalletEvents(t *testing.T) {
	repo := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, repo)

	ctx := context.Background()

	wallet := &Wallet{
		UserID:   "user-events",
		Currency: "USD",
		Balance:  "0.0000",
		Status:   StatusActive,
	}

	created, err := repo.CreateWallet(ctx, wallet)
	if err != nil {
		t.Fatalf("Failed to create wallet: %v", err)
	}

	// Create multiple events
	events := []WalletEvent{
		{
			WalletID:      created.ID,
			EventType:     EventTypeDeposit,
			Amount:        "50.0000",
			BalanceBefore: "0.0000",
			BalanceAfter:  "50.0000",
		},
		{
			WalletID:      created.ID,
			EventType:     EventTypeDeposit,
			Amount:        "25.0000",
			BalanceBefore: "50.0000",
			BalanceAfter:  "75.0000",
		},
	}

	for _, e := range events {
		_, err := repo.CreateWalletEvent(ctx, &e)
		if err != nil {
			t.Fatalf("Failed to create event: %v", err)
		}
	}

	// Get events
	foundEvents, err := repo.GetWalletEvents(ctx, created.ID, 10, 0)
	if err != nil {
		t.Fatalf("Failed to get events: %v", err)
	}

	if len(foundEvents) != 2 {
		t.Errorf("Expected 2 events, got %d", len(foundEvents))
	}
}