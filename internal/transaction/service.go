package transaction

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"time"

	"github.com/kmassidik/mercuria/internal/common/db"
	"github.com/kmassidik/mercuria/internal/common/kafka"
	"github.com/kmassidik/mercuria/internal/common/logger"
	"github.com/kmassidik/mercuria/internal/common/redis"
	"github.com/kmassidik/mercuria/pkg/outbox"
)

type Service struct {
	repo       *Repository
	outboxRepo *outbox.Repository
	redis      *redis.Client
	producer   *kafka.Producer
	db         *db.DB
	logger     *logger.Logger
}

func NewService(
	repo *Repository,
	outboxRepo *outbox.Repository,
	redisClient *redis.Client,
	producer *kafka.Producer,
	database *db.DB,
	log *logger.Logger,
) *Service {
	return &Service{
		repo:       repo,
		outboxRepo: outboxRepo,
		redis:      redisClient,
		producer:   producer,
		db:         database,
		logger:     log,
	}
}

func (s *Service) CreateP2PTransfer(ctx context.Context, req *CreateTransactionRequest) (*Transaction, error) {
    // 1. Validate request
    if err := ValidateCreateTransactionRequest(req); err != nil {
        return nil, fmt.Errorf("validation failed: %w", err)
    }

    // 2. Check idempotency (Redis)
    exists, err := s.redis.CheckIdempotency(ctx, req.IdempotencyKey)
    if err != nil {
        return nil, fmt.Errorf("idempotency check failed: %w", err)
    }
    if exists {
        return nil, fmt.Errorf("duplicate request: idempotency key already used")
    }

    // 3. Execute transfer in transaction
    var completedTxn *Transaction
    err = s.db.WithTransaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
        // 3a. Get and validate wallets
        fromWallet, err := s.getWalletForUpdate(ctx, tx, req.FromWalletID)
        if err != nil {
            return fmt.Errorf("source wallet error: %w", err)
        }

        toWallet, err := s.getWalletForUpdate(ctx, tx, req.ToWalletID)
        if err != nil {
            return fmt.Errorf("destination wallet error: %w", err)
        }

        // 3b. Validate currencies match
        if fromWallet.Currency != toWallet.Currency {
            return fmt.Errorf("currency mismatch: %s != %s", fromWallet.Currency, toWallet.Currency)
        }

        // 3c. Check sufficient balance
        if !hasSufficientBalance(fromWallet.Balance, req.Amount) {
            return fmt.Errorf("insufficient balance")
        }

        // 3d. Calculate new balances
        newFromBalance, err := subtractAmounts(fromWallet.Balance, req.Amount)
        if err != nil {
            return fmt.Errorf("failed to calculate from balance: %w", err)
        }

        newToBalance, err := addAmounts(toWallet.Balance, req.Amount)
        if err != nil {
            return fmt.Errorf("failed to calculate to balance: %w", err)
        }

        // 3e. Update balances
        if err := s.updateWalletBalance(ctx, tx, req.FromWalletID, newFromBalance); err != nil {
            return fmt.Errorf("failed to update source wallet: %w", err)
        }

        if err := s.updateWalletBalance(ctx, tx, req.ToWalletID, newToBalance); err != nil {
            return fmt.Errorf("failed to update destination wallet: %w", err)
        }

        // 3f. Create transaction record
        txn := &Transaction{
            FromWalletID:   req.FromWalletID,
            ToWalletID:     req.ToWalletID,
            Amount:         req.Amount,
            Currency:       fromWallet.Currency,
            Type:           TypeP2P,
            Status:         StatusCompleted,
            Description:    req.Description,
            IdempotencyKey: req.IdempotencyKey,
        }

        createdTxn, err := s.repo.CreateTransactionTx(ctx, tx, txn)
        if err != nil {
            return fmt.Errorf("failed to create transaction: %w", err)
        }
        completedTxn = createdTxn

        // 3g. Save outbox event
        event := &outbox.OutboxEvent{
            AggregateID: createdTxn.ID,
            EventType:   "transaction.completed",
            Topic:       "transaction.completed",
            Payload: map[string]interface{}{
                "transaction_id": createdTxn.ID,
                "from_wallet_id": req.FromWalletID,
                "to_wallet_id":   req.ToWalletID,
                "amount":         req.Amount,
                "currency":       fromWallet.Currency,
                "type":           TypeP2P,
                "completed_at":   time.Now(),
            },
        }

        if err := s.outboxRepo.SaveEvent(ctx, tx, event); err != nil {
            return fmt.Errorf("failed to save outbox event: %w", err)
        }

        return nil
    })

    if err != nil {
        s.logger.Errorf("Transfer failed: %v", err)
        return nil, err
    }

    // 4. Set idempotency key (after successful commit)
    if err := s.redis.SetIdempotency(ctx, req.IdempotencyKey, 30*time.Minute); err != nil {
        s.logger.Warnf("Failed to set idempotency key: %v", err)
    }

    s.logger.Infof("P2P transfer completed: %s", completedTxn.ID)
    return completedTxn, nil
}

func (s *Service) CreateBatchTransfer(ctx context.Context, req *CreateBatchTransactionRequest) (*BatchTransaction, []Transaction, error) {
    // 1. Validate request
    if err := ValidateCreateBatchTransactionRequest(req); err != nil {
        return nil, nil, fmt.Errorf("validation failed: %w", err)
    }

    // 2. Check idempotency
    exists, err := s.redis.CheckIdempotency(ctx, req.IdempotencyKey)
    if err != nil {
        return nil, nil, fmt.Errorf("idempotency check failed: %w", err)
    }
    if exists {
        return nil, nil, fmt.Errorf("duplicate request: idempotency key already used")
    }

    // 3. Calculate total amount
    totalAmount, err := CalculateBatchTotal(req.Transfers)
    if err != nil {
        return nil, nil, fmt.Errorf("failed to calculate total: %w", err)
    }

    // 4. Execute batch in transaction
    var batch *BatchTransaction
    var transactions []Transaction

    err = s.db.WithTransaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
        // 4a. Get source wallet
        fromWallet, err := s.getWalletForUpdate(ctx, tx, req.FromWalletID)
        if err != nil {
            return fmt.Errorf("source wallet error: %w", err)
        }

        // 4b. Check sufficient balance for total
        if !hasSufficientBalance(fromWallet.Balance, totalAmount) {
            return fmt.Errorf("insufficient balance for batch (need %s, have %s)", totalAmount, fromWallet.Balance)
        }

        // 4c. Create batch record
        batchTxn := &BatchTransaction{
            FromWalletID:   req.FromWalletID,
            TotalAmount:    totalAmount,
            Currency:       fromWallet.Currency,
            Status:         StatusPending,
            IdempotencyKey: req.IdempotencyKey,
            Transfers:      req.Transfers,
        }

        createdBatch, err := s.repo.CreateBatchTransactionTx(ctx, tx, batchTxn)
        if err != nil {
            return fmt.Errorf("failed to create batch: %w", err)
        }
        batch = createdBatch

        // 4d. Process each transfer
        currentBalance := fromWallet.Balance
        for i, transfer := range req.Transfers {
            // Get recipient wallet
            toWallet, err := s.getWalletForUpdate(ctx, tx, transfer.ToWalletID)
            if err != nil {
                return fmt.Errorf("transfer[%d] recipient error: %w", i, err)
            }

            // Validate currency
            if fromWallet.Currency != toWallet.Currency {
                return fmt.Errorf("transfer[%d] currency mismatch", i)
            }

            // Calculate new balances
            newFromBalance, err := subtractAmounts(currentBalance, transfer.Amount)
            if err != nil {
                return fmt.Errorf("transfer[%d] balance calculation failed: %w", i, err)
            }
            currentBalance = newFromBalance

            newToBalance, err := addAmounts(toWallet.Balance, transfer.Amount)
            if err != nil {
                return fmt.Errorf("transfer[%d] to balance calculation failed: %w", i, err)
            }

            // Update recipient balance
            if err := s.updateWalletBalance(ctx, tx, transfer.ToWalletID, newToBalance); err != nil {
                return fmt.Errorf("transfer[%d] update recipient failed: %w", i, err)
            }

            // Create individual transaction record
            txn := &Transaction{
                FromWalletID:   req.FromWalletID,
                ToWalletID:     transfer.ToWalletID,
                Amount:         transfer.Amount,
                Currency:       fromWallet.Currency,
                Type:           TypeBatch,
                Status:         StatusCompleted,
                Description:    fmt.Sprintf("Batch transfer [%d/%d]: %s", i+1, len(req.Transfers), transfer.Description),
                IdempotencyKey: fmt.Sprintf("%s-item-%d", req.IdempotencyKey, i),
            }

            createdTxn, err := s.repo.CreateTransactionTx(ctx, tx, txn)
            if err != nil {
                return fmt.Errorf("transfer[%d] create transaction failed: %w", i, err)
            }
            transactions = append(transactions, *createdTxn)
        }

        // 4e. Update source wallet with final balance
        if err := s.updateWalletBalance(ctx, tx, req.FromWalletID, currentBalance); err != nil {
            return fmt.Errorf("failed to update source wallet: %w", err)
        }

        // 4f. Update batch status
        batch.Status = StatusCompleted

        // 4g. Save outbox event
        event := &outbox.OutboxEvent{
            AggregateID: batch.ID,
            EventType:   "batch.completed",
            Topic:       "transaction.completed",
            Payload: map[string]interface{}{
                "batch_id":      batch.ID,
                "from_wallet_id": req.FromWalletID,
                "total_amount":  totalAmount,
                "count":         len(req.Transfers),
                "completed_at":  time.Now(),
            },
        }

        if err := s.outboxRepo.SaveEvent(ctx, tx, event); err != nil {
            return fmt.Errorf("failed to save outbox event: %w", err)
        }

        return nil
    })

    if err != nil {
        s.logger.Errorf("Batch transfer failed: %v", err)
        return nil, nil, err
    }

    // 5. Set idempotency key
    if err := s.redis.SetIdempotency(ctx, req.IdempotencyKey, 30*time.Minute); err != nil {
        s.logger.Warnf("Failed to set idempotency key: %v", err)
    }

    s.logger.Infof("Batch transfer completed: %s (%d transfers)", batch.ID, len(transactions))
    return batch, transactions, nil
}

// CreateScheduledTransfer creates a future-dated transfer
// NOTE: Transfer is created but not executed until scheduled time
func (s *Service) CreateScheduledTransfer(ctx context.Context, req *CreateScheduledTransactionRequest) (*Transaction, error) {
	// 1. Validate request
	if err := ValidateCreateScheduledTransactionRequest(req); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// 2. Check idempotency
	exists, err := s.redis.CheckIdempotency(ctx, req.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("idempotency check failed: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("duplicate request: idempotency key already used")
	}

	// 3. Verify wallets exist and currencies match
	fromWallet, err := s.getWallet(ctx, req.FromWalletID)
	if err != nil {
		return nil, fmt.Errorf("source wallet error: %w", err)
	}

	toWallet, err := s.getWallet(ctx, req.ToWalletID)
	if err != nil {
		return nil, fmt.Errorf("destination wallet error: %w", err)
	}

	if fromWallet.Currency != toWallet.Currency {
		return nil, fmt.Errorf("currency mismatch")
	}

	// 4. Create scheduled transaction (not executed yet)
	txn := &Transaction{
		FromWalletID:   req.FromWalletID,
		ToWalletID:     req.ToWalletID,
		Amount:         req.Amount,
		Currency:       fromWallet.Currency,
		Type:           TypeScheduled,
		Status:         StatusScheduled,
		Description:    req.Description,
		IdempotencyKey: req.IdempotencyKey,
		ScheduledAt:    &req.ScheduledAt,
	}

	createdTxn, err := s.repo.CreateTransaction(ctx, txn)
	if err != nil {
		return nil, fmt.Errorf("failed to create scheduled transaction: %w", err)
	}

	// 5. Set idempotency key
	if err := s.redis.SetIdempotency(ctx, req.IdempotencyKey, 30*time.Minute); err != nil {
		s.logger.Warnf("Failed to set idempotency key: %v", err)
	}

	s.logger.Infof("Scheduled transfer created: %s (scheduled for %s)", createdTxn.ID, req.ScheduledAt)
	return createdTxn, nil
}

// ProcessScheduledTransfers executes transfers that are due
// NOTE: Called by background worker, processes scheduled transactions
func (s *Service) ProcessScheduledTransfers(ctx context.Context) (int, error) {
	// 1. Get scheduled transactions that are due
	scheduled, err := s.repo.GetScheduledTransactions(ctx, 100)
	if err != nil {
		return 0, fmt.Errorf("failed to get scheduled transactions: %w", err)
	}

	if len(scheduled) == 0 {
		return 0, nil
	}

	s.logger.Infof("Processing %d scheduled transfers", len(scheduled))

	processed := 0
	for _, txn := range scheduled {
		// Execute the scheduled transfer
		err := s.executeScheduledTransfer(ctx, &txn)
		if err != nil {
			s.logger.Errorf("Failed to execute scheduled transfer %s: %v", txn.ID, err)
			// Mark as failed
			s.repo.MarkTransactionAsFailed(ctx, txn.ID, err.Error())
			continue
		}
		processed++
	}

	s.logger.Infof("Processed %d/%d scheduled transfers", processed, len(scheduled))
	return processed, nil
}

func (s *Service) executeScheduledTransfer(ctx context.Context, txn *Transaction) error {
    return s.db.WithTransaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
        // Get wallets
        fromWallet, err := s.getWalletForUpdate(ctx, tx, txn.FromWalletID)
        if err != nil {
            return err
        }

        toWallet, err := s.getWalletForUpdate(ctx, tx, txn.ToWalletID)
        if err != nil {
            return err
        }

        // Check balance
        if !hasSufficientBalance(fromWallet.Balance, txn.Amount) {
            return fmt.Errorf("insufficient balance")
        }

        // Calculate new balances
        newFromBalance, _ := subtractAmounts(fromWallet.Balance, txn.Amount)
        newToBalance, _ := addAmounts(toWallet.Balance, txn.Amount)

        // Update balances
        s.updateWalletBalance(ctx, tx, txn.FromWalletID, newFromBalance)
        s.updateWalletBalance(ctx, tx, txn.ToWalletID, newToBalance)

        // Update transaction status
        s.repo.UpdateTransactionStatusTx(ctx, tx, txn.ID, StatusCompleted)

        // Save outbox event
        event := &outbox.OutboxEvent{
            AggregateID: txn.ID,
            EventType:   "transaction.completed",
            Topic:       "transaction.completed",
            Payload: map[string]interface{}{
                "transaction_id": txn.ID,
                "from_wallet_id": txn.FromWalletID,
                "to_wallet_id":   txn.ToWalletID,
                "amount":         txn.Amount,
                "type":           TypeScheduled,
                "completed_at":   time.Now(),
            },
        }
        return s.outboxRepo.SaveEvent(ctx, tx, event)
    })
}

func (s *Service) getWallet(ctx context.Context, id string) (*WalletInfo, error) {
	var wallet WalletInfo
	query := `SELECT id, user_id, currency, balance, status FROM wallets WHERE id = $1`
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&wallet.ID, &wallet.UserID, &wallet.Currency, &wallet.Balance, &wallet.Status,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("wallet not found")
	}
	if err != nil {
		return nil, err
	}
	return &wallet, nil
}

// getWalletForUpdate retrieves wallet with row lock
func (s *Service) getWalletForUpdate(ctx context.Context, tx *sql.Tx, id string) (*WalletInfo, error) {
	var wallet WalletInfo
	query := `SELECT id, user_id, currency, balance, status FROM wallets WHERE id = $1 FOR UPDATE`
	err := tx.QueryRowContext(ctx, query, id).Scan(
		&wallet.ID, &wallet.UserID, &wallet.Currency, &wallet.Balance, &wallet.Status,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("wallet not found")
	}
	if err != nil {
		return nil, err
	}
	if wallet.Status != "active" {
		return nil, fmt.Errorf("wallet is not active")
	}
	return &wallet, nil
}

// updateWalletBalance updates wallet balance within transaction
func (s *Service) updateWalletBalance(ctx context.Context, tx *sql.Tx, id, newBalance string) error {
	query := `UPDATE wallets SET balance = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`
	_, err := tx.ExecContext(ctx, query, newBalance, id)
	return err
}

type WalletInfo struct {
	ID       string
	UserID   string
	Currency string
	Balance  string
	Status   string
}

// Decimal arithmetic helpers

func addAmounts(a, b string) (string, error) {
	aVal := new(big.Float)
	bVal := new(big.Float)
	if _, ok := aVal.SetString(a); !ok {
		return "", fmt.Errorf("invalid amount: %s", a)
	}
	if _, ok := bVal.SetString(b); !ok {
		return "", fmt.Errorf("invalid amount: %s", b)
	}
	result := new(big.Float).Add(aVal, bVal)
	return result.Text('f', 4), nil
}

func subtractAmounts(a, b string) (string, error) {
	aVal := new(big.Float)
	bVal := new(big.Float)
	if _, ok := aVal.SetString(a); !ok {
		return "", fmt.Errorf("invalid amount: %s", a)
	}
	if _, ok := bVal.SetString(b); !ok {
		return "", fmt.Errorf("invalid amount: %s", b)
	}
	result := new(big.Float).Sub(aVal, bVal)
	return result.Text('f', 4), nil
}

func hasSufficientBalance(balance, amount string) bool {
	balanceVal := new(big.Float)
	amountVal := new(big.Float)
	balanceVal.SetString(balance)
	amountVal.SetString(amount)
	return balanceVal.Cmp(amountVal) >= 0
}

// GetTransaction retrieves a transaction by ID
func (s *Service) GetTransaction(ctx context.Context, id string) (*Transaction, error) {
	// 1. Retrieve transaction from repository
	txn, err := s.repo.GetTransaction(ctx, id)
	if err != nil {
		// The repository returns "transaction not found" error, which will be logged
		return nil, err
	}

	// NOTE: You would perform the "Verify user owns one of the wallets in transaction" TODO here
	
	return txn, nil
}

// ListTransactionsByWallet lists transactions for a specific wallet
func (s *Service) ListTransactionsByWallet(ctx context.Context, walletID string, limit, offset int) ([]Transaction, error) {
	// NOTE: You would perform the "Verify user owns wallet" TODO here

	// 1. Retrieve transactions from repository
	txns, err := s.repo.ListTransactionsByWallet(ctx, walletID, limit, offset)
	if err != nil {
		return nil, err
	}
	
	return txns, nil
}