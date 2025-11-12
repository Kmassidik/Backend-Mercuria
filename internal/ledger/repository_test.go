package ledger

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
		DBName:          "mercuria_ledger_test",
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
	CREATE TABLE IF NOT EXISTS ledger_entries (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		transaction_id VARCHAR(255) NOT NULL,
		wallet_id VARCHAR(255) NOT NULL,
		entry_type VARCHAR(10) NOT NULL,
		amount NUMERIC(20, 4) NOT NULL,
		currency VARCHAR(3) NOT NULL,
		balance NUMERIC(20, 4) NOT NULL,
		description TEXT,
		metadata JSONB,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		CHECK (entry_type IN ('debit', 'credit')),
		CHECK (amount > 0)
	);

	CREATE INDEX IF NOT EXISTS idx_ledger_transaction ON ledger_entries(transaction_id);
	CREATE INDEX IF NOT EXISTS idx_ledger_wallet ON ledger_entries(wallet_id, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_ledger_created ON ledger_entries(created_at DESC);

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

	TRUNCATE ledger_entries, outbox_events CASCADE;
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
	database.Exec("TRUNCATE ledger_entries, outbox_events CASCADE")
	database.Close()
}

// TEST: Create ledger entry
func TestCreateLedgerEntry(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()

	entry := &LedgerEntry{
		TransactionID: "txn-123",
		WalletID:      "wallet-456",
		EntryType:     EntryTypeDebit,
		Amount:        "100.50",
		Currency:      "USD",
		Balance:       "400.00",
		Description:   "Test debit",
		Metadata: map[string]interface{}{
			"note": "test entry",
		},
	}

	created, err := repo.CreateLedgerEntry(ctx, entry)
	if err != nil {
		t.Fatalf("Failed to create ledger entry: %v", err)
	}

	if created.ID == "" {
		t.Error("Expected entry ID to be set")
	}

	if created.EntryType != EntryTypeDebit {
		t.Errorf("Expected entry type debit, got %s", created.EntryType)
	}
}

// TEST: Get ledger entry by ID
func TestGetLedgerEntry(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()

	entry := &LedgerEntry{
		TransactionID: "txn-789",
		WalletID:      "wallet-999",
		EntryType:     EntryTypeCredit,
		Amount:        "50.00",
		Currency:      "USD",
		Balance:       "150.00",
		Description:   "Test credit",
	}

	created, _ := repo.CreateLedgerEntry(ctx, entry)

	// Get entry
	found, err := repo.GetLedgerEntry(ctx, created.ID)
	if err != nil {
		t.Fatalf("Failed to get ledger entry: %v", err)
	}

	if found.ID != created.ID {
		t.Errorf("Expected entry ID %s, got %s", created.ID, found.ID)
	}

	if found.Amount != "50.0000" {
		t.Errorf("Expected amount 50.0000, got %s", found.Amount)
	}
}

// TEST: Get entries by transaction ID
func TestGetEntriesByTransaction(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()

	txnID := "txn-multi"

	// Create debit entry
	debit := &LedgerEntry{
		TransactionID: txnID,
		WalletID:      "wallet-from",
		EntryType:     EntryTypeDebit,
		Amount:        "100.00",
		Currency:      "USD",
		Balance:       "400.00",
	}
	repo.CreateLedgerEntry(ctx, debit)

	// Create credit entry
	credit := &LedgerEntry{
		TransactionID: txnID,
		WalletID:      "wallet-to",
		EntryType:     EntryTypeCredit,
		Amount:        "100.00",
		Currency:      "USD",
		Balance:       "200.00",
	}
	repo.CreateLedgerEntry(ctx, credit)

	// Get entries for transaction
	entries, err := repo.GetEntriesByTransaction(ctx, txnID)
	if err != nil {
		t.Fatalf("Failed to get entries: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(entries))
	}

	// Verify we have one debit and one credit
	hasDebit := false
	hasCredit := false
	for _, e := range entries {
		if e.EntryType == EntryTypeDebit {
			hasDebit = true
		}
		if e.EntryType == EntryTypeCredit {
			hasCredit = true
		}
	}

	if !hasDebit || !hasCredit {
		t.Error("Expected one debit and one credit entry")
	}
}

// TEST: Get entries by wallet
func TestGetEntriesByWallet(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()

	walletID := "wallet-history"

	// Create multiple entries
	for i := 0; i < 3; i++ {
		entry := &LedgerEntry{
			TransactionID: "txn-" + string(rune(i)),
			WalletID:      walletID,
			EntryType:     EntryTypeCredit,
			Amount:        "50.00",
			Currency:      "USD",
			Balance:       "100.00",
		}
		repo.CreateLedgerEntry(ctx, entry)
	}

	// Get entries
	entries, err := repo.GetEntriesByWallet(ctx, walletID, 10, 0)
	if err != nil {
		t.Fatalf("Failed to get wallet entries: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(entries))
	}
}

// TEST: Get latest balance for wallet
func TestGetLatestBalance(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()

	walletID := "wallet-balance"

	// Create entries with different balances
	entries := []string{"100.0000", "150.0000", "200.0000"}
	for _, balance := range entries {
		entry := &LedgerEntry{
			TransactionID: "txn-balance",
			WalletID:      walletID,
			EntryType:     EntryTypeCredit,
			Amount:        "50.00",
			Currency:      "USD",
			Balance:       balance,
		}
		repo.CreateLedgerEntry(ctx, entry)
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}

	// Get latest balance
	balance, err := repo.GetLatestBalance(ctx, walletID)
	if err != nil {
		t.Fatalf("Failed to get latest balance: %v", err)
	}

	if balance != "200.0000" {
		t.Errorf("Expected balance 200.0000, got %s", balance)
	}
}

// TEST: Verify double-entry balance
func TestVerifyDoubleEntryBalance(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()

	txnID := "txn-balanced"

	// Create balanced entries
	debit := &LedgerEntry{
		TransactionID: txnID,
		WalletID:      "wallet-from",
		EntryType:     EntryTypeDebit,
		Amount:        "100.00",
		Currency:      "USD",
		Balance:       "400.00",
	}
	repo.CreateLedgerEntry(ctx, debit)

	credit := &LedgerEntry{
		TransactionID: txnID,
		WalletID:      "wallet-to",
		EntryType:     EntryTypeCredit,
		Amount:        "100.00",
		Currency:      "USD",
		Balance:       "200.00",
	}
	repo.CreateLedgerEntry(ctx, credit)

	// Verify balanced
	balanced, err := repo.VerifyTransactionBalance(ctx, txnID)
	if err != nil {
		t.Fatalf("Failed to verify balance: %v", err)
	}

	if !balanced {
		t.Error("Expected transaction to be balanced")
	}
}

// TEST: Unbalanced transaction detection
func TestUnbalancedTransaction(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()

	txnID := "txn-unbalanced"

	// Create unbalanced entries (different amounts)
	debit := &LedgerEntry{
		TransactionID: txnID,
		WalletID:      "wallet-from",
		EntryType:     EntryTypeDebit,
		Amount:        "100.00",
		Currency:      "USD",
		Balance:       "400.00",
	}
	repo.CreateLedgerEntry(ctx, debit)

	credit := &LedgerEntry{
		TransactionID: txnID,
		WalletID:      "wallet-to",
		EntryType:     EntryTypeCredit,
		Amount:        "50.00", // Different amount!
		Currency:      "USD",
		Balance:       "200.00",
	}
	repo.CreateLedgerEntry(ctx, credit)

	// Verify unbalanced
	balanced, err := repo.VerifyTransactionBalance(ctx, txnID)
	if err != nil {
		t.Fatalf("Failed to verify balance: %v", err)
	}

	if balanced {
		t.Error("Expected transaction to be unbalanced")
	}
}

// TEST: Get ledger statistics
func TestGetLedgerStats(t *testing.T) {
	repo, database := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, database)

	ctx := context.Background()

	walletID := "wallet-stats"

	// Create debits
	for i := 0; i < 2; i++ {
		entry := &LedgerEntry{
			TransactionID: "txn-debit",
			WalletID:      walletID,
			EntryType:     EntryTypeDebit,
			Amount:        "50.00",
			Currency:      "USD",
			Balance:       "100.00",
		}
		repo.CreateLedgerEntry(ctx, entry)
	}

	// Create credits
	for i := 0; i < 3; i++ {
		entry := &LedgerEntry{
			TransactionID: "txn-credit",
			WalletID:      walletID,
			EntryType:     EntryTypeCredit,
			Amount:        "30.00",
			Currency:      "USD",
			Balance:       "100.00",
		}
		repo.CreateLedgerEntry(ctx, entry)
	}

	// Get stats
	stats, err := repo.GetWalletStats(ctx, walletID)
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	if stats.EntryCount != 5 {
		t.Errorf("Expected 5 entries, got %d", stats.EntryCount)
	}

	// Total debits: 2 * 50 = 100
	if stats.TotalDebits != "100.0000" {
		t.Errorf("Expected total debits 100.0000, got %s", stats.TotalDebits)
	}

	// Total credits: 3 * 30 = 90
	if stats.TotalCredits != "90.0000" {
		t.Errorf("Expected total credits 90.0000, got %s", stats.TotalCredits)
	}
}