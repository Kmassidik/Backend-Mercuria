package wallet

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

// CreateWallet creates a new wallet
func (r *Repository) CreateWallet(ctx context.Context, wallet *Wallet) (*Wallet, error) {
	query := `
		INSERT INTO wallets (user_id, currency, balance, status)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowContext(
		ctx,
		query,
		wallet.UserID,
		wallet.Currency,
		wallet.Balance,
		wallet.Status,
	).Scan(&wallet.ID, &wallet.CreatedAt, &wallet.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	r.logger.Infof("Wallet created: %s for user %s", wallet.ID, wallet.UserID)
	return wallet, nil
}

// GetWallet retrieves a wallet by ID
func (r *Repository) GetWallet(ctx context.Context, id string) (*Wallet, error) {
	query := `
		SELECT id, user_id, currency, balance, status, created_at, updated_at
		FROM wallets
		WHERE id = $1
	`

	wallet := &Wallet{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&wallet.ID,
		&wallet.UserID,
		&wallet.Currency,
		&wallet.Balance,
		&wallet.Status,
		&wallet.CreatedAt,
		&wallet.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("wallet not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}

	return wallet, nil
}

// GetWalletByUserAndCurrency retrieves a wallet by user ID and currency
func (r *Repository) GetWalletByUserAndCurrency(ctx context.Context, userID, currency string) (*Wallet, error) {
	query := `
		SELECT id, user_id, currency, balance, status, created_at, updated_at
		FROM wallets
		WHERE user_id = $1 AND currency = $2
	`

	wallet := &Wallet{}
	err := r.db.QueryRowContext(ctx, query, userID, currency).Scan(
		&wallet.ID,
		&wallet.UserID,
		&wallet.Currency,
		&wallet.Balance,
		&wallet.Status,
		&wallet.CreatedAt,
		&wallet.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("wallet not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}

	return wallet, nil
}

// UpdateBalance updates wallet balance
func (r *Repository) UpdateBalance(ctx context.Context, walletID, newBalance string) error {
	query := `
		UPDATE wallets
		SET balance = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`

	result, err := r.db.ExecContext(ctx, query, newBalance, walletID)
	if err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("wallet not found")
	}

	return nil
}

// UpdateBalanceWithLock updates balance with row-level lock
func (r *Repository) UpdateBalanceWithLock(ctx context.Context, tx *sql.Tx, walletID, newBalance string) error {
	query := `
		UPDATE wallets
		SET balance = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`

	result, err := tx.ExecContext(ctx, query, newBalance, walletID)
	if err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("wallet not found")
	}

	return nil
}

// GetWalletForUpdate locks wallet row for update (SELECT FOR UPDATE)
func (r *Repository) GetWalletForUpdate(ctx context.Context, tx *sql.Tx, walletID string) (*Wallet, error) {
	query := `
		SELECT id, user_id, currency, balance, status, created_at, updated_at
		FROM wallets
		WHERE id = $1
		FOR UPDATE
	`

	wallet := &Wallet{}
	err := tx.QueryRowContext(ctx, query, walletID).Scan(
		&wallet.ID,
		&wallet.UserID,
		&wallet.Currency,
		&wallet.Balance,
		&wallet.Status,
		&wallet.CreatedAt,
		&wallet.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("wallet not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}

	return wallet, nil
}

// CreateWalletEvent creates a wallet event
func (r *Repository) CreateWalletEvent(ctx context.Context, event *WalletEvent) (*WalletEvent, error) {
	var metadataJSON []byte
	var err error

	if event.Metadata != nil {
		metadataJSON, err = json.Marshal(event.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	query := `
		INSERT INTO wallet_events (wallet_id, event_type, amount, balance_before, balance_after, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`

	err = r.db.QueryRowContext(
		ctx,
		query,
		event.WalletID,
		event.EventType,
		event.Amount,
		event.BalanceBefore,
		event.BalanceAfter,
		metadataJSON,
	).Scan(&event.ID, &event.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create wallet event: %w", err)
	}

	return event, nil
}

// CreateWalletEventTx creates a wallet event within a transaction
func (r *Repository) CreateWalletEventTx(ctx context.Context, tx *sql.Tx, event *WalletEvent) (*WalletEvent, error) {
	var metadataJSON []byte
	var err error

	if event.Metadata != nil {
		metadataJSON, err = json.Marshal(event.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	query := `
		INSERT INTO wallet_events (wallet_id, event_type, amount, balance_before, balance_after, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`

	err = tx.QueryRowContext(
		ctx,
		query,
		event.WalletID,
		event.EventType,
		event.Amount,
		event.BalanceBefore,
		event.BalanceAfter,
		metadataJSON,
	).Scan(&event.ID, &event.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create wallet event: %w", err)
	}

	return event, nil
}

// GetWalletEvents retrieves wallet events with pagination
func (r *Repository) GetWalletEvents(ctx context.Context, walletID string, limit, offset int) ([]WalletEvent, error) {
	query := `
		SELECT id, wallet_id, event_type, amount, balance_before, balance_after, metadata, created_at
		FROM wallet_events
		WHERE wallet_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, walletID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet events: %w", err)
	}
	defer rows.Close()

	var events []WalletEvent
	for rows.Next() {
		var event WalletEvent
		var metadataJSON []byte

		err := rows.Scan(
			&event.ID,
			&event.WalletID,
			&event.EventType,
			&event.Amount,
			&event.BalanceBefore,
			&event.BalanceAfter,
			&metadataJSON,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}

		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &event.Metadata); err != nil {
				r.logger.Warnf("Failed to unmarshal metadata: %v", err)
			}
		}

		events = append(events, event)
	}

	return events, nil
}

func (r *Repository) GetWalletTx(ctx context.Context, tx *sql.Tx, id string) (*Wallet, error) {
    query := `
        SELECT id, user_id, currency, balance, status, created_at, updated_at
        FROM wallets
        WHERE id = $1
    `

    wallet := &Wallet{}
    // CRITICAL: Use tx.QueryRowContext instead of r.db.QueryRowContext
    err := tx.QueryRowContext(ctx, query, id).Scan( 
        &wallet.ID,
        &wallet.UserID,
        &wallet.Currency,
        &wallet.Balance,
        &wallet.Status,
        &wallet.CreatedAt,
        &wallet.UpdatedAt,
    )

    if err == sql.ErrNoRows {
        return nil, fmt.Errorf("wallet not found")
    }
    if err != nil {
        return nil, fmt.Errorf("failed to get wallet from transaction: %w", err)
    }

    return wallet, nil
}