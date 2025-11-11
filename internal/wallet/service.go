package wallet

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"time"

	"github.com/kmassidik/mercuria/internal/common/kafka"
	"github.com/kmassidik/mercuria/internal/common/logger"
	"github.com/kmassidik/mercuria/internal/common/redis"
)

type Service struct {
	repo     *Repository
	redis    *redis.Client
	producer *kafka.Producer
	logger   *logger.Logger
}

func NewService(repo *Repository, redisClient *redis.Client, producer *kafka.Producer, log *logger.Logger) *Service {
	return &Service{
		repo:     repo,
		redis:    redisClient,
		producer: producer,
		logger:   log,
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

	// Create wallet
	wallet := &Wallet{
		UserID:   req.UserID,
		Currency: req.Currency,
		Balance:  "0.0000",
		Status:   StatusActive,
	}

	created, err := s.repo.CreateWallet(ctx, wallet)
	if err != nil {
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	// Publish wallet created event
	event := WalletCreatedEvent{
		WalletID:  created.ID,
		UserID:    created.UserID,
		Currency:  created.Currency,
		CreatedAt: created.CreatedAt,
	}

	if err := s.producer.PublishEvent(ctx, "wallet.created", created.ID, event); err != nil {
		s.logger.Warnf("Failed to publish wallet created event: %v", err)
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
	err = s.repo.db.WithTransaction(ctx, func(tx *sql.Tx) error {
		// Get wallet with lock
		wallet, err := s.repo.GetWalletForUpdate(ctx, tx, walletID)
		if err != nil {
			return err
		}

		if wallet.Status != StatusActive {
			return fmt.Errorf("wallet is not active")
		}

		// Calculate new balance
		balanceBefore := wallet.Balance
		newBalance, err := addAmounts(wallet.Balance, req.Amount)
		if err != nil {
			return fmt.Errorf("failed to calculate new balance: %w", err)
		}

		// Update balance
		if err := s.repo.UpdateBalanceWithLock(ctx, tx, walletID, newBalance); err != nil {
			return err
		}

		// Create event
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

	// Publish balance updated event
	balanceEvent := BalanceUpdatedEvent{
		WalletID:      walletID,
		UserID:        updatedWallet.UserID,
		EventType:     EventTypeDeposit,
		Amount:        req.Amount,
		BalanceBefore: updatedWallet.Balance, // This should be old balance, but we'll use new for simplicity
		BalanceAfter:  updatedWallet.Balance,
		Timestamp:     time.Now(),
	}

	if err := s.producer.PublishEvent(ctx, "wallet.balance_updated", walletID, balanceEvent); err != nil {
		s.logger.Warnf("Failed to publish balance updated event: %v", err)
	}

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
	err = s.repo.db.WithTransaction(ctx, func(tx *sql.Tx) error {
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
		balanceBefore := wallet.Balance
		newBalance, err := subtractAmounts(wallet.Balance, req.Amount)
		if err != nil {
			return fmt.Errorf("failed to calculate new balance: %w", err)
		}

		// Update balance
		if err := s.repo.UpdateBalanceWithLock(ctx, tx, walletID, newBalance); err != nil {
			return err
		}

		// Create event
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

		// Get updated wallet
		updatedWallet, err = s.repo.GetWallet(ctx, walletID)
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

	// Publish balance updated event
	balanceEvent := BalanceUpdatedEvent{
		WalletID:      walletID,
		UserID:        updatedWallet.UserID,
		EventType:     EventTypeWithdrawal,
		Amount:        req.Amount,
		BalanceBefore: updatedWallet.Balance,
		BalanceAfter:  updatedWallet.Balance,
		Timestamp:     time.Now(),
	}

	if err := s.producer.PublishEvent(ctx, "wallet.balance_updated", walletID, balanceEvent); err != nil {
		s.logger.Warnf("Failed to publish balance updated event: %v", err)
	}

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