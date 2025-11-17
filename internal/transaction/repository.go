package transaction

import (
	"context"
	"database/sql"
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

// CreateTransaction creates a new transaction record
// NOTE: This only creates the DB record, doesn't execute the transfer
func (r *Repository) CreateTransaction(ctx context.Context, txn *Transaction) (*Transaction, error) {
	query := `
		INSERT INTO transactions (
			from_wallet_id, to_wallet_id, amount, currency, type, 
			status, description, idempotency_key, scheduled_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowContext(
		ctx,
		query,
		txn.FromWalletID,
		txn.ToWalletID,
		txn.Amount,
		txn.Currency,
		txn.Type,
		txn.Status,
		txn.Description,
		txn.IdempotencyKey,
		txn.ScheduledAt,
	).Scan(&txn.ID, &txn.CreatedAt, &txn.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	r.logger.Infof("Transaction created: %s", txn.ID)
	return txn, nil
}

// CreateTransactionTx creates a transaction within an existing DB transaction
// NOTE: Used when creating transaction + outbox event atomically
func (r *Repository) CreateTransactionTx(ctx context.Context, tx *sql.Tx, txn *Transaction) (*Transaction, error) {
	query := `
		INSERT INTO transactions (
			from_wallet_id, to_wallet_id, amount, currency, type, 
			status, description, idempotency_key, scheduled_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at
	`

	err := tx.QueryRowContext(
		ctx,
		query,
		txn.FromWalletID,
		txn.ToWalletID,
		txn.Amount,
		txn.Currency,
		txn.Type,
		txn.Status,
		txn.Description,
		txn.IdempotencyKey,
		txn.ScheduledAt,
	).Scan(&txn.ID, &txn.CreatedAt, &txn.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	return txn, nil
}

// GetTransaction retrieves a transaction by ID
func (r *Repository) GetTransaction(ctx context.Context, id string) (*Transaction, error) {
	query := `
		SELECT 
			id, from_wallet_id, to_wallet_id, amount, currency, type,
			status, description, idempotency_key, scheduled_at, 
			processed_at, failure_reason, created_at, updated_at
		FROM transactions
		WHERE id = $1
	`

	txn := &Transaction{}
	var failureReason sql.NullString // <- ADD THIS
	
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&txn.ID,
		&txn.FromWalletID,
		&txn.ToWalletID,
		&txn.Amount,
		&txn.Currency,
		&txn.Type,
		&txn.Status,
		&txn.Description,
		&txn.IdempotencyKey,
		&txn.ScheduledAt,
		&txn.ProcessedAt,
		&failureReason, // <- CHANGE THIS
		&txn.CreatedAt,
		&txn.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("transaction not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	// Convert sql.NullString to *string
	if failureReason.Valid {
		txn.FailureReason = &failureReason.String
	}

	return txn, nil
}

// UpdateTransactionStatus updates transaction status and processed_at
// NOTE: Used when transaction completes or fails
func (r *Repository) UpdateTransactionStatus(ctx context.Context, id string, status string) error {
	query := `
		UPDATE transactions
		SET status = $1, processed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`

	result, err := r.db.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update transaction status: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("transaction not found")
	}

	r.logger.Infof("Transaction %s status updated to %s", id, status)
	return nil
}

// UpdateTransactionStatusTx updates status within a transaction
// NOTE: Used when updating status + creating outbox event atomically
func (r *Repository) UpdateTransactionStatusTx(ctx context.Context, tx *sql.Tx, id string, status string) error {
	query := `
		UPDATE transactions
		SET status = $1, processed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`

	result, err := tx.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update transaction status: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("transaction not found")
	}

	return nil
}

// MarkTransactionAsFailed marks a transaction as failed with reason
func (r *Repository) MarkTransactionAsFailed(ctx context.Context, id string, reason string) error {
	query := `
		UPDATE transactions
		SET status = $1, failure_reason = $2, processed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3
	`

	result, err := r.db.ExecContext(ctx, query, StatusFailed, reason, id)
	if err != nil {
		return fmt.Errorf("failed to mark transaction as failed: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("transaction not found")
	}

	r.logger.Warnf("Transaction %s marked as failed: %s", id, reason)
	return nil
}

// GetScheduledTransactions retrieves transactions that are due to be processed
// NOTE: Called by background worker to execute scheduled transfers
func (r *Repository) GetScheduledTransactions(ctx context.Context, limit int) ([]Transaction, error) {
	query := `
		SELECT 
			id, from_wallet_id, to_wallet_id, amount, currency, type,
			status, description, idempotency_key, scheduled_at, 
			created_at, updated_at
		FROM transactions
		WHERE status = $1 AND scheduled_at <= CURRENT_TIMESTAMP
		ORDER BY scheduled_at ASC
		LIMIT $2
	`

	rows, err := r.db.QueryContext(ctx, query, StatusScheduled, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get scheduled transactions: %w", err)
	}
	defer rows.Close()

	var transactions []Transaction
	for rows.Next() {
		var txn Transaction
		err := rows.Scan(
			&txn.ID,
			&txn.FromWalletID,
			&txn.ToWalletID,
			&txn.Amount,
			&txn.Currency,
			&txn.Type,
			&txn.Status,
			&txn.Description,
			&txn.IdempotencyKey,
			&txn.ScheduledAt,
			&txn.CreatedAt,
			&txn.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}
		transactions = append(transactions, txn)
	}

	return transactions, nil
}

// ListTransactionsByWallet lists transactions for a wallet (sent or received)
// NOTE: Used for transaction history API
func (r *Repository) ListTransactionsByWallet(ctx context.Context, walletID string, limit, offset int) ([]Transaction, error) {
	query := `
		SELECT 
			id, from_wallet_id, to_wallet_id, amount, currency, type,
			status, description, idempotency_key, scheduled_at, 
			processed_at, failure_reason, created_at, updated_at
		FROM transactions
		WHERE from_wallet_id = $1 OR to_wallet_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, walletID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list transactions: %w", err)
	}
	defer rows.Close()

	var transactions []Transaction
	for rows.Next() {
		var txn Transaction
		var failureReason sql.NullString // <- ADD THIS
		
		err := rows.Scan(
			&txn.ID,
			&txn.FromWalletID,
			&txn.ToWalletID,
			&txn.Amount,
			&txn.Currency,
			&txn.Type,
			&txn.Status,
			&txn.Description,
			&txn.IdempotencyKey,
			&txn.ScheduledAt,
			&txn.ProcessedAt,
			&failureReason, // <- CHANGE THIS
			&txn.CreatedAt,
			&txn.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}
		
		// Convert sql.NullString to *string
		if failureReason.Valid {
			txn.FailureReason = &failureReason.String
		}
		
		transactions = append(transactions, txn)
	}

	return transactions, nil
}

// CreateBatchTransaction creates a batch transaction record
// NOTE: Individual transfers are stored as separate Transaction records
func (r *Repository) CreateBatchTransaction(ctx context.Context, batch *BatchTransaction) (*BatchTransaction, error) {
	query := `
		INSERT INTO batch_transactions (
			from_wallet_id, total_amount, currency, status, idempotency_key
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowContext(
		ctx,
		query,
		batch.FromWalletID,
		batch.TotalAmount,
		batch.Currency,
		batch.Status,
		batch.IdempotencyKey,
	).Scan(&batch.ID, &batch.CreatedAt, &batch.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create batch transaction: %w", err)
	}

	r.logger.Infof("Batch transaction created: %s", batch.ID)
	return batch, nil
}

// CreateBatchTransactionTx creates batch within existing transaction
func (r *Repository) CreateBatchTransactionTx(ctx context.Context, tx *sql.Tx, batch *BatchTransaction) (*BatchTransaction, error) {
	query := `
		INSERT INTO batch_transactions (
			from_wallet_id, total_amount, currency, status, idempotency_key
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`

	err := tx.QueryRowContext(
		ctx,
		query,
		batch.FromWalletID,
		batch.TotalAmount,
		batch.Currency,
		batch.Status,
		batch.IdempotencyKey,
	).Scan(&batch.ID, &batch.CreatedAt, &batch.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create batch transaction: %w", err)
	}

	return batch, nil
}

// GetBatchTransaction retrieves a batch transaction by ID
func (r *Repository) GetBatchTransaction(ctx context.Context, id string) (*BatchTransaction, error) {
	query := `
		SELECT 
			id, from_wallet_id, total_amount, currency, status,
			idempotency_key, created_at, updated_at
		FROM batch_transactions
		WHERE id = $1
	`

	batch := &BatchTransaction{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&batch.ID,
		&batch.FromWalletID,
		&batch.TotalAmount,
		&batch.Currency,
		&batch.Status,
		&batch.IdempotencyKey,
		&batch.CreatedAt,
		&batch.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("batch transaction not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get batch transaction: %w", err)
	}

	return batch, nil
}

// UpdateBatchTransactionStatusTx updates batch status within a transaction
func (r *Repository) UpdateBatchTransactionStatusTx(ctx context.Context, tx *sql.Tx, id string, status string) error {
	query := `
		UPDATE batch_transactions
		SET status = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`

	result, err := tx.ExecContext(ctx, query, status, id)  // ← Use tx, not r.db
	if err != nil {
		return fmt.Errorf("failed to update batch status: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("batch transaction not found")
	}

	return nil
}