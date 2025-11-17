package wallet

import (
	"context"
	"database/sql"
	"encoding/json"
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

func NewService(repo *Repository, outboxRepo *outbox.Repository, redisClient *redis.Client, producer *kafka.Producer, database *db.DB, log *logger.Logger) *Service {
	return &Service{
		repo:       repo,
		outboxRepo: outboxRepo,
		redis:      redisClient,
		producer:   producer,
		db:         database,
		logger:     log,
	}
}

// CreateWallet creates a new wallet
func (s *Service) CreateWallet(ctx context.Context, req *CreateWalletRequest) (*Wallet, error) {
	// Validate request
	if err := ValidateCreateWalletRequest(req); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check if wallet already exists
	existing, _ := s.repo.GetWalletByUserAndCurrency(ctx, req.UserID, req.Currency)
	if existing != nil {
		return nil, fmt.Errorf("wallet already exists for user %s with currency %s", req.UserID, req.Currency)
	}

	var created *Wallet

	// Use transaction for atomicity
	err := s.db.WithTransaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// Create wallet
		wallet := &Wallet{
			UserID:   req.UserID,
			Currency: req.Currency,
			Balance:  "0.0000",
			Status:   StatusActive,
		}

		// Insert wallet
		query := `
			INSERT INTO wallets (user_id, currency, balance, status)
			VALUES ($1, $2, $3, $4)
			RETURNING id, created_at, updated_at
		`
		err := tx.QueryRowContext(
			ctx,
			query,
			wallet.UserID,
			wallet.Currency,
			wallet.Balance,
			wallet.Status,
		).Scan(&wallet.ID, &wallet.CreatedAt, &wallet.UpdatedAt)

		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}

		created = wallet

		// Save event to outbox (same transaction)
		eventData := WalletCreatedEvent{
			WalletID:  created.ID,
			UserID:    created.UserID,
			Currency:  created.Currency,
			CreatedAt: created.CreatedAt,
		}

		// Convert event to map for outbox
		eventBytes, _ := json.Marshal(eventData)
		var eventMap map[string]interface{}
		json.Unmarshal(eventBytes, &eventMap)

		outboxEvent := &outbox.OutboxEvent{
			AggregateID: created.ID,
			EventType:   "wallet.created",
			Topic:       "wallet.created",
			Payload:     eventMap,
		}

		if err := s.outboxRepo.SaveEvent(ctx, tx, outboxEvent); err != nil {
			return fmt.Errorf("failed to save outbox event: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	s.logger.Infof("Wallet created: %s for user %s", created.ID, created.UserID)
	return created, nil
}

// GetWallet retrieves a wallet (with caching)
func (s *Service) GetWallet(ctx context.Context, walletID string) (*Wallet, error) {
	// Try to get from cache
	cachedBalance, err := s.redis.GetCachedWalletBalance(ctx, walletID)
	if err == nil && cachedBalance != "" {
		// Get wallet from DB but use cached balance
		wallet, err := s.repo.GetWallet(ctx, walletID)
		if err != nil {
			return nil, err
		}
		wallet.Balance = cachedBalance
		return wallet, nil
	}

	// Cache miss - get from DB
	wallet, err := s.repo.GetWallet(ctx, walletID)
	if err != nil {
		return nil, err
	}

	// Cache the balance
	if err := s.redis.CacheWalletBalance(ctx, walletID, wallet.Balance, 10*time.Minute); err != nil {
		s.logger.Warnf("Failed to cache balance: %v", err)
	}

	return wallet, nil
}

// Deposit adds funds to a wallet
func (s *Service) Deposit(ctx context.Context, walletID string, req *DepositRequest) (*Wallet, error) {
	// Validate request
	if err := ValidateDepositRequest(req); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check idempotency
	exists, err := s.redis.CheckIdempotency(ctx, req.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to check idempotency: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("duplicate request: idempotency key already used")
	}

	// Acquire wallet lock
	lockKey := fmt.Sprintf("wallet:%s", walletID)
	locked, err := s.redis.AcquireLock(ctx, lockKey, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("wallet is locked, please try again")
	}
	defer s.redis.ReleaseLock(ctx, lockKey)

	// Start transaction
	var updatedWallet *Wallet
	var balanceBefore, balanceAfter string

	err = s.db.WithTransaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// Get wallet with lock
		wallet, err := s.repo.GetWalletForUpdate(ctx, tx, walletID)
		if err != nil {
			return err
		}

		if wallet.Status != StatusActive {
			return fmt.Errorf("wallet is not active")
		}

		// Calculate new balance
		balanceBefore = wallet.Balance
		newBalance, err := addAmounts(wallet.Balance, req.Amount)
		if err != nil {
			return fmt.Errorf("failed to calculate new balance: %w", err)
		}
		balanceAfter = newBalance

		// Update balance
		if err := s.repo.UpdateBalanceWithLock(ctx, tx, walletID, newBalance); err != nil {
			return err
		}

		// Create wallet event
		event := &WalletEvent{
			WalletID:      walletID,
			EventType:     EventTypeDeposit,
			Amount:        req.Amount,
			BalanceBefore: balanceBefore,
			BalanceAfter:  newBalance,
			Metadata: map[string]interface{}{
				"description":     req.Description,
				"idempotency_key": req.IdempotencyKey,
			},
		}

		if _, err := s.repo.CreateWalletEventTx(ctx, tx, event); err != nil {
			return err
		}

		// Save balance updated event to outbox
		balanceEventData := BalanceUpdatedEvent{
			WalletID:      walletID,
			UserID:        wallet.UserID,
			EventType:     EventTypeDeposit,
			Amount:        req.Amount,
			BalanceBefore: balanceBefore,
			BalanceAfter:  balanceAfter,
			Timestamp:     time.Now(),
		}

		// Convert event to map for outbox
		eventBytes, _ := json.Marshal(balanceEventData)
		var eventMap map[string]interface{}
		json.Unmarshal(eventBytes, &eventMap)

		outboxEvent := &outbox.OutboxEvent{
			AggregateID: walletID,
			EventType:   "wallet.balance_updated",
			Topic:       "wallet.balance_updated",
			Payload:     eventMap,
		}

		if err := s.outboxRepo.SaveEvent(ctx, tx, outboxEvent); err != nil {
			return fmt.Errorf("failed to save outbox event: %w", err)
		}

		// Get updated wallet
		updatedWallet, err = s.repo.GetWallet(ctx, walletID)
		return err
	})

	if err != nil {
		return nil, fmt.Errorf("deposit failed: %w", err)
	}

	// Set idempotency key
	if err := s.redis.SetIdempotency(ctx, req.IdempotencyKey, 30*time.Minute); err != nil {
		s.logger.Warnf("Failed to set idempotency key: %v", err)
	}

	// Invalidate cache
	s.redis.InvalidateWalletBalance(ctx, walletID)

	s.logger.Infof("Deposit successful: %s to wallet %s", req.Amount, walletID)
	return updatedWallet, nil
}

// Withdraw removes funds from a wallet
func (s *Service) Withdraw(ctx context.Context, walletID string, req *WithdrawRequest) (*Wallet, error) {
	// Validate request
	if err := ValidateWithdrawRequest(req); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check idempotency
	exists, err := s.redis.CheckIdempotency(ctx, req.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to check idempotency: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("duplicate request: idempotency key already used")
	}

	// Acquire wallet lock
	lockKey := fmt.Sprintf("wallet:%s", walletID)
	locked, err := s.redis.AcquireLock(ctx, lockKey, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("wallet is locked, please try again")
	}
	defer s.redis.ReleaseLock(ctx, lockKey)

	// Start transaction
	var updatedWallet *Wallet
	var balanceBefore, balanceAfter string

	err = s.db.WithTransaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// Get wallet with lock
		wallet, err := s.repo.GetWalletForUpdate(ctx, tx, walletID)
		if err != nil {
			return err
		}

		if wallet.Status != StatusActive {
			return fmt.Errorf("wallet is not active")
		}

		// Check sufficient balance
		if !hasSufficientBalance(wallet.Balance, req.Amount) {
			return fmt.Errorf("insufficient balance")
		}

		// Calculate new balance
		balanceBefore = wallet.Balance
		newBalance, err := subtractAmounts(wallet.Balance, req.Amount)
		if err != nil {
			return fmt.Errorf("failed to calculate new balance: %w", err)
		}
		balanceAfter = newBalance

		// Update balance
		if err := s.repo.UpdateBalanceWithLock(ctx, tx, walletID, newBalance); err != nil {
			return err
		}

		// Create wallet event
		event := &WalletEvent{
			WalletID:      walletID,
			EventType:     EventTypeWithdrawal,
			Amount:        req.Amount,
			BalanceBefore: balanceBefore,
			BalanceAfter:  newBalance,
			Metadata: map[string]interface{}{
				"description":     req.Description,
				"idempotency_key": req.IdempotencyKey,
			},
		}

		if _, err := s.repo.CreateWalletEventTx(ctx, tx, event); err != nil {
			return err
		}

		// Save balance updated event to outbox
		balanceEventData := BalanceUpdatedEvent{
			WalletID:      walletID,
			UserID:        wallet.UserID,
			EventType:     EventTypeWithdrawal,
			Amount:        req.Amount,
			BalanceBefore: balanceBefore,
			BalanceAfter:  balanceAfter,
			Timestamp:     time.Now(),
		}

		// Convert event to map for outbox
		eventBytes, _ := json.Marshal(balanceEventData)
		var eventMap map[string]interface{}
		json.Unmarshal(eventBytes, &eventMap)

		outboxEvent := &outbox.OutboxEvent{
			AggregateID: walletID,
			EventType:   "wallet.balance_updated",
			Topic:       "wallet.balance_updated",
			Payload:     eventMap,
		}

		if err := s.outboxRepo.SaveEvent(ctx, tx, outboxEvent); err != nil {
			return fmt.Errorf("failed to save outbox event: %w", err)
		}

		// Get updated wallet
		// updatedWallet, err = s.repo.GetWallet(ctx, walletID)
		updatedWallet, err = s.repo.GetWalletTx(ctx, tx, walletID)
		return err
	})

	if err != nil {
		return nil, fmt.Errorf("withdrawal failed: %w", err)
	}

	// Set idempotency key
	if err := s.redis.SetIdempotency(ctx, req.IdempotencyKey, 30*time.Minute); err != nil {
		s.logger.Warnf("Failed to set idempotency key: %v", err)
	}

	// Invalidate cache
	s.redis.InvalidateWalletBalance(ctx, walletID)

	s.logger.Infof("Withdrawal successful: %s from wallet %s", req.Amount, walletID)
	return updatedWallet, nil
}

// GetWalletEvents retrieves wallet transaction history
func (s *Service) GetWalletEvents(ctx context.Context, walletID string, limit, offset int) ([]WalletEvent, error) {
	events, err := s.repo.GetWalletEvents(ctx, walletID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet events: %w", err)
	}

	return events, nil
}

// Helper functions for decimal arithmetic
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

// GetWalletsByUserID retrieves all wallets for a user
func (s *Service) GetWalletsByUserID(ctx context.Context, userID string) ([]Wallet, error) {
	wallets, err := s.repo.GetWalletsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallets: %w", err)
	}
	return wallets, nil
}

// Transfer moves funds between two wallets atomically
func (s *Service) Transfer(ctx context.Context, req *TransferRequest) error {
	// 1. Validate amount
	if req.Amount == "" || req.IdempotencyKey == "" {
		return fmt.Errorf("amount and idempotency_key are required")
	}

	// 2. Check idempotency
	exists, err := s.redis.CheckIdempotency(ctx, req.IdempotencyKey)
	if err != nil {
		return fmt.Errorf("idempotency check failed: %w", err)
	}
	if exists {
		return fmt.Errorf("duplicate request: idempotency key already used")
	}

	// 3. Acquire locks for both wallets (prevent deadlock by ordering)
	walletIDs := []string{req.FromWalletID, req.ToWalletID}
	if req.FromWalletID > req.ToWalletID {
		walletIDs = []string{req.ToWalletID, req.FromWalletID}
	}

	for _, id := range walletIDs {
		lockKey := fmt.Sprintf("wallet:%s", id)
		locked, err := s.redis.AcquireLock(ctx, lockKey, 5*time.Second)
		if err != nil {
			return fmt.Errorf("failed to acquire lock: %w", err)
		}
		if !locked {
			return fmt.Errorf("wallet is locked, please try again")
		}
		defer s.redis.ReleaseLock(ctx, lockKey)
	}

	// 4. Execute transfer in transaction
	err = s.db.WithTransaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// Get source wallet with lock
		fromWallet, err := s.repo.GetWalletForUpdate(ctx, tx, req.FromWalletID)
		if err != nil {
			return fmt.Errorf("source wallet error: %w", err)
		}

		// Get destination wallet with lock
		toWallet, err := s.repo.GetWalletForUpdate(ctx, tx, req.ToWalletID)
		if err != nil {
			return fmt.Errorf("destination wallet error: %w", err)
		}

		// Validate wallets are active
		if fromWallet.Status != StatusActive {
			return fmt.Errorf("source wallet is not active")
		}
		if toWallet.Status != StatusActive {
			return fmt.Errorf("destination wallet is not active")
		}

		// Check sufficient balance
		if !hasSufficientBalance(fromWallet.Balance, req.Amount) {
			return fmt.Errorf("insufficient balance")
		}

		// Calculate new balances
		newFromBalance, err := subtractAmounts(fromWallet.Balance, req.Amount)
		if err != nil {
			return fmt.Errorf("failed to calculate source balance: %w", err)
		}

		newToBalance, err := addAmounts(toWallet.Balance, req.Amount)
		if err != nil {
			return fmt.Errorf("failed to calculate destination balance: %w", err)
		}

		// Update source wallet balance
		if err := s.repo.UpdateBalanceWithLock(ctx, tx, req.FromWalletID, newFromBalance); err != nil {
			return fmt.Errorf("failed to update source wallet: %w", err)
		}

		// Update destination wallet balance
		if err := s.repo.UpdateBalanceWithLock(ctx, tx, req.ToWalletID, newToBalance); err != nil {
			return fmt.Errorf("failed to update destination wallet: %w", err)
		}

		// Create event for source wallet (withdrawal)
		fromEvent := &WalletEvent{
			WalletID:      req.FromWalletID,
			EventType:     "wallet.transfer_out",
			Amount:        req.Amount,
			BalanceBefore: fromWallet.Balance,
			BalanceAfter:  newFromBalance,
			Metadata: map[string]interface{}{
				"to_wallet_id":    req.ToWalletID,
				"idempotency_key": req.IdempotencyKey,
			},
		}
		if _, err := s.repo.CreateWalletEventTx(ctx, tx, fromEvent); err != nil {
			return fmt.Errorf("failed to create source event: %w", err)
		}

		// Create event for destination wallet (deposit)
		toEvent := &WalletEvent{
			WalletID:      req.ToWalletID,
			EventType:     "wallet.transfer_in",
			Amount:        req.Amount,
			BalanceBefore: toWallet.Balance,
			BalanceAfter:  newToBalance,
			Metadata: map[string]interface{}{
				"from_wallet_id":  req.FromWalletID,
				"idempotency_key": req.IdempotencyKey,
			},
		}
		if _, err := s.repo.CreateWalletEventTx(ctx, tx, toEvent); err != nil {
			return fmt.Errorf("failed to create destination event: %w", err)
		}

		// Publish balance updated events to outbox
		fromBalanceEvent := BalanceUpdatedEvent{
			WalletID:      req.FromWalletID,
			UserID:        fromWallet.UserID,
			EventType:     "wallet.transfer_out",
			Amount:        req.Amount,
			BalanceBefore: fromWallet.Balance,
			BalanceAfter:  newFromBalance,
			Timestamp:     time.Now(),
		}

		fromEventBytes, _ := json.Marshal(fromBalanceEvent)
		var fromEventMap map[string]interface{}
		json.Unmarshal(fromEventBytes, &fromEventMap)

		fromOutboxEvent := &outbox.OutboxEvent{
			AggregateID: req.FromWalletID,
			EventType:   "wallet.balance_updated",
			Topic:       "wallet.balance_updated",
			Payload:     fromEventMap,
		}

		if err := s.outboxRepo.SaveEvent(ctx, tx, fromOutboxEvent); err != nil {
			return fmt.Errorf("failed to save source outbox event: %w", err)
		}

		toBalanceEvent := BalanceUpdatedEvent{
			WalletID:      req.ToWalletID,
			UserID:        toWallet.UserID,
			EventType:     "wallet.transfer_in",
			Amount:        req.Amount,
			BalanceBefore: toWallet.Balance,
			BalanceAfter:  newToBalance,
			Timestamp:     time.Now(),
		}

		toEventBytes, _ := json.Marshal(toBalanceEvent)
		var toEventMap map[string]interface{}
		json.Unmarshal(toEventBytes, &toEventMap)

		toOutboxEvent := &outbox.OutboxEvent{
			AggregateID: req.ToWalletID,
			EventType:   "wallet.balance_updated",
			Topic:       "wallet.balance_updated",
			Payload:     toEventMap,
		}

		if err := s.outboxRepo.SaveEvent(ctx, tx, toOutboxEvent); err != nil {
			return fmt.Errorf("failed to save destination outbox event: %w", err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("transfer failed: %w", err)
	}

	// Set idempotency key after successful transfer
	if err := s.redis.SetIdempotency(ctx, req.IdempotencyKey, 30*time.Minute); err != nil {
		s.logger.Warnf("Failed to set idempotency key: %v", err)
	}

	// Invalidate cache for both wallets
	s.redis.InvalidateWalletBalance(ctx, req.FromWalletID)
	s.redis.InvalidateWalletBalance(ctx, req.ToWalletID)

	s.logger.Infof("Transfer completed: %s from %s to %s", req.Amount, req.FromWalletID, req.ToWalletID)
	return nil
}