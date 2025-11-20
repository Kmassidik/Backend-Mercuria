package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/kmassidik/mercuria/internal/common/db"
	"github.com/kmassidik/mercuria/internal/common/logger"
)

type Repository struct {
	db     *db.DB
	logger *logger.Logger
}

func NewRepository(database *db.DB, log *logger.Logger) *Repository {
	return &Repository{
		db:     database,
		logger: log,
	}
}

// CreateLedgerEntry creates a new ledger entry
// NOTE: Ledger entries are IMMUTABLE - no updates or deletes allowed
func (r *Repository) CreateLedgerEntry(ctx context.Context, entry *LedgerEntry) (*LedgerEntry, error) {
	var metadataJSON []byte
	var err error

	if entry.Metadata != nil {
		metadataJSON, err = json.Marshal(entry.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	query := `
		INSERT INTO ledger_entries (
			transaction_id, wallet_id, entry_type, amount, currency, 
			balance, description, metadata
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at
	`

	err = r.db.QueryRowContext(
		ctx,
		query,
		entry.TransactionID,
		entry.WalletID,
		entry.EntryType,
		entry.Amount,
		entry.Currency,
		entry.Balance,
		entry.Description,
		metadataJSON,
	).Scan(&entry.ID, &entry.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create ledger entry: %w", err)
	}

	r.logger.Infof("Ledger entry created: %s for transaction %s", entry.ID, entry.TransactionID)
	return entry, nil
}

// CreateLedgerEntryTx creates entry within existing transaction
// NOTE: Used when creating entry + outbox event atomically
func (r *Repository) CreateLedgerEntryTx(ctx context.Context, tx *sql.Tx, entry *LedgerEntry) (*LedgerEntry, error) {
	var metadataJSON []byte
	var err error

	if entry.Metadata != nil {
		metadataJSON, err = json.Marshal(entry.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	query := `
		INSERT INTO ledger_entries (
			transaction_id, wallet_id, entry_type, amount, currency, 
			balance, description, metadata
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at
	`

	err = tx.QueryRowContext(
		ctx,
		query,
		entry.TransactionID,
		entry.WalletID,
		entry.EntryType,
		entry.Amount,
		entry.Currency,
		entry.Balance,
		entry.Description,
		metadataJSON,
	).Scan(&entry.ID, &entry.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create ledger entry: %w", err)
	}

	return entry, nil
}

// GetLedgerEntry - Fix NULL metadata handling
func (r *Repository) GetLedgerEntry(ctx context.Context, id string) (*LedgerEntry, error) {
	query := `
		SELECT 
			id, transaction_id, wallet_id, entry_type, amount, currency,
			balance, description, metadata, created_at
		FROM ledger_entries
		WHERE id = $1
	`

	entry := &LedgerEntry{}
	var metadataJSON []byte

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&entry.ID,
		&entry.TransactionID,
		&entry.WalletID,
		&entry.EntryType,
		&entry.Amount,
		&entry.Currency,
		&entry.Balance,
		&entry.Description,
		&metadataJSON,
		&entry.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("ledger entry not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get ledger entry: %w", err)
	}

	// Safely unmarshal metadata (handle NULL)
	if len(metadataJSON) > 0 && string(metadataJSON) != "null" {
		if err := json.Unmarshal(metadataJSON, &entry.Metadata); err != nil {
			r.logger.Warnf("Failed to unmarshal metadata for entry %s: %v", id, err)
			entry.Metadata = make(map[string]interface{}) // Initialize empty map
		}
	} else {
		entry.Metadata = make(map[string]interface{}) // Initialize empty map
	}

	return entry, nil
}

// GetEntriesByTransaction retrieves all ledger entries for a transaction
// NOTE: Should return 2+ entries (debit + credit) for double-entry bookkeeping
func (r *Repository) GetEntriesByTransaction(ctx context.Context, transactionID string) ([]LedgerEntry, error) {
	query := `
		SELECT 
			id, transaction_id, wallet_id, entry_type, amount, currency,
			balance, description, metadata, created_at
		FROM ledger_entries
		WHERE transaction_id = $1
		ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get entries: %w", err)
	}
	defer rows.Close()

	return r.scanEntries(rows)
}

// GetEntriesByWallet retrieves ledger entries for a wallet with pagination
// NOTE: Returns entries in reverse chronological order (newest first)
func (r *Repository) GetEntriesByWallet(ctx context.Context, walletID string, limit, offset int) ([]LedgerEntry, error) {
	query := `
		SELECT 
			id, transaction_id, wallet_id, entry_type, amount, currency,
			balance, description, metadata, created_at
		FROM ledger_entries
		WHERE wallet_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, walletID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet entries: %w", err)
	}
	defer rows.Close()

	return r.scanEntries(rows)
}

// GetLatestBalance retrieves the most recent balance for a wallet
// NOTE: Gets the balance field from the newest ledger entry
func (r *Repository) GetLatestBalance(ctx context.Context, walletID string) (string, error) {
	query := `
		SELECT balance
		FROM ledger_entries
		WHERE wallet_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`

	var balance string
	err := r.db.QueryRowContext(ctx, query, walletID).Scan(&balance)
	if err == sql.ErrNoRows {
		return "0.0000", nil // No entries yet
	}
	if err != nil {
		return "", fmt.Errorf("failed to get latest balance: %w", err)
	}

	return balance, nil
}

// VerifyTransactionBalance verifies double-entry bookkeeping for a transaction
// NOTE: Total debits must equal total credits
func (r *Repository) VerifyTransactionBalance(ctx context.Context, transactionID string) (bool, error) {
	query := `
		SELECT 
			COALESCE(SUM(CASE WHEN entry_type = 'debit' THEN amount ELSE 0 END), 0) as total_debits,
			COALESCE(SUM(CASE WHEN entry_type = 'credit' THEN amount ELSE 0 END), 0) as total_credits
		FROM ledger_entries
		WHERE transaction_id = $1
	`

	var totalDebits, totalCredits string
	err := r.db.QueryRowContext(ctx, query, transactionID).Scan(&totalDebits, &totalCredits)
	if err != nil {
		return false, fmt.Errorf("failed to verify balance: %w", err)
	}

	// Check if debits equal credits
	balanced := totalDebits == totalCredits

	if !balanced {
		r.logger.Warnf("Transaction %s is unbalanced: debits=%s, credits=%s", transactionID, totalDebits, totalCredits)
	}

	return balanced, nil
}

// GetWalletStats calculates statistics for a wallet
// NOTE: Useful for analytics and reporting
func (r *Repository) GetWalletStats(ctx context.Context, walletID string) (*LedgerStats, error) {
	query := `
		SELECT 
			COALESCE(SUM(CASE WHEN entry_type = 'debit' THEN amount ELSE 0 END), 0) as total_debits,
			COALESCE(SUM(CASE WHEN entry_type = 'credit' THEN amount ELSE 0 END), 0) as total_credits,
			COUNT(*) as entry_count,
			MIN(created_at) as first_entry,
			MAX(created_at) as last_entry
		FROM ledger_entries
		WHERE wallet_id = $1
	`

	stats := &LedgerStats{WalletID: walletID}
	err := r.db.QueryRowContext(ctx, query, walletID).Scan(
		&stats.TotalDebits,
		&stats.TotalCredits,
		&stats.EntryCount,
		&stats.FirstEntry,
		&stats.LastEntry,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	// Calculate net change (credits - debits)
	// TODO: Use proper decimal arithmetic
	stats.NetChange = "0.0000" // Placeholder

	return stats, nil
}

// GetAllEntriesPaginated retrieves all ledger entries with pagination
// NOTE: For admin/audit purposes
func (r *Repository) GetAllEntriesPaginated(ctx context.Context, limit, offset int) ([]LedgerEntry, error) {
	query := `
		SELECT 
			id, transaction_id, wallet_id, entry_type, amount, currency,
			balance, description, metadata, created_at
		FROM ledger_entries
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get entries: %w", err)
	}
	defer rows.Close()

	return r.scanEntries(rows)
}

func (r *Repository) scanEntries(rows *sql.Rows) ([]LedgerEntry, error) {
	var entries []LedgerEntry

	for rows.Next() {
		var entry LedgerEntry
		var metadataJSON []byte

		err := rows.Scan(
			&entry.ID,
			&entry.TransactionID,
			&entry.WalletID,
			&entry.EntryType,
			&entry.Amount,
			&entry.Currency,
			&entry.Balance,
			&entry.Description,
			&metadataJSON,
			&entry.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan entry: %w", err)
		}

		// Safely unmarshal metadata (handle NULL)
		if len(metadataJSON) > 0 && string(metadataJSON) != "null" {
			if err := json.Unmarshal(metadataJSON, &entry.Metadata); err != nil {
				r.logger.Warnf("Failed to unmarshal metadata: %v", err)
				entry.Metadata = make(map[string]interface{})
			}
		} else {
			entry.Metadata = make(map[string]interface{})
		}

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return entries, nil
}