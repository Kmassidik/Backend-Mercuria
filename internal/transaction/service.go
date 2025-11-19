package transaction

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"time"

	"github.com/kmassidik/mercuria/internal/common/db"
	"github.com/kmassidik/mercuria/internal/common/kafka"
	"github.com/kmassidik/mercuria/internal/common/logger"
	"github.com/kmassidik/mercuria/internal/common/mtls"
	"github.com/kmassidik/mercuria/internal/common/redis"
	"github.com/kmassidik/mercuria/pkg/outbox"
)

type Service struct {
	repo          *Repository
	outboxRepo    *outbox.Repository
	redis         *redis.Client
	producer      *kafka.Producer
	db            *db.DB
	logger        *logger.Logger
	walletBaseURL string
	httpClient    *http.Client
}

func NewService(
	repo *Repository,
	outboxRepo *outbox.Repository,
	redisClient *redis.Client,
	producer *kafka.Producer,
	database *db.DB,
	log *logger.Logger,
) *Service {
	// Use environment variable or default to localhost
	walletServiceURL := "http://localhost:8081"
	if url := os.Getenv("WALLET_SERVICE_URL"); url != "" {
		walletServiceURL = url
	}

	// Load mTLS configuration
	mtlsConfig := mtls.LoadFromEnv()
	
	// Create HTTP client
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Configure mTLS for client if enabled
    if mtlsConfig.Enabled {
        tlsConfig, err := mtlsConfig.ClientTLSConfig()
        if err != nil {
            log.Fatalf("Failed to load mTLS client config: %v", err)
        }
        
        httpClient.Transport = &http.Transport{
            TLSClientConfig: tlsConfig,
        }
        
        log.Info("✅ mTLS client configuration loaded")
    }

	return &Service{
		repo:          repo,
		outboxRepo:    outboxRepo,
		redis:         redisClient,
		producer:      producer,
		db:            database,
		logger:        log,
		walletBaseURL: walletServiceURL,
		 httpClient: httpClient,
	}
}

func (s *Service) getWalletFromService(ctx context.Context, walletID string) (*WalletInfo, error) {
	url := fmt.Sprintf("%s/api/v1/internal/wallets/%s", s.walletBaseURL, walletID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Note: With mTLS, authentication happens at TLS layer
	// No need to forward JWT for internal calls
	
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wallet service unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("wallet not found")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("wallet service error: %s", string(body))
	}

	var response struct {
		Wallet *WalletInfo `json:"wallet"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if response.Wallet.Status != "active" {
		return nil, fmt.Errorf("wallet is not active")
	}

	return response.Wallet, nil
}

// executeWalletTransfer calls Wallet Service to execute the actual transfer
func (s *Service) executeWalletTransfer(ctx context.Context, req *WalletTransferRequest) error {
	url := fmt.Sprintf("%s/api/v1/internal/wallets/transfer", s.walletBaseURL)

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Note: With mTLS, authentication happens at TLS layer
	// No need to forward JWT for internal calls

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("wallet service unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("transfer failed: %s", string(bodyBytes))
	}

	return nil
}

// WalletTransferRequest for calling Wallet Service transfer API
type WalletTransferRequest struct {
	FromWalletID   string `json:"from_wallet_id"`
	ToWalletID     string `json:"to_wallet_id"`
	Amount         string `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
}

type WalletInfo struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	Currency string `json:"currency"`
	Balance  string `json:"balance"`
	Status   string `json:"status"`
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

	// 3. Get wallet info from Wallet Service (not from local DB)
	fromWallet, err := s.getWalletFromService(ctx, req.FromWalletID)
	if err != nil {
		return nil, fmt.Errorf("source wallet error: %w", err)
	}

	toWallet, err := s.getWalletFromService(ctx, req.ToWalletID)
	if err != nil {
		return nil, fmt.Errorf("destination wallet error: %w", err)
	}

	// 4. Validate currencies match
	if fromWallet.Currency != toWallet.Currency {
		return nil, fmt.Errorf("currency mismatch: %s != %s", fromWallet.Currency, toWallet.Currency)
	}

	// 5. Check sufficient balance
	if !hasSufficientBalance(fromWallet.Balance, req.Amount) {
		return nil, fmt.Errorf("insufficient balance")
	}

	// 6. Execute transfer via Wallet Service API (not local DB update)
	transferReq := WalletTransferRequest{
		FromWalletID:   req.FromWalletID,
		ToWalletID:     req.ToWalletID,
		Amount:         req.Amount,
		IdempotencyKey: req.IdempotencyKey,
	}

	if err := s.executeWalletTransfer(ctx, &transferReq); err != nil {
		return nil, fmt.Errorf("wallet transfer failed: %w", err)
	}

	// 7. Create transaction record in local DB
	var completedTxn *Transaction
	err = s.db.WithTransaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
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

		// 8. Save outbox event
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

	// 9. Set idempotency key (after successful commit)
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

	// 4. Get source wallet from Wallet Service
	fromWallet, err := s.getWalletFromService(ctx, req.FromWalletID)
	if err != nil {
		return nil, nil, fmt.Errorf("source wallet error: %w", err)
	}

	// 5. Check sufficient balance for total
	if !hasSufficientBalance(fromWallet.Balance, totalAmount) {
		return nil, nil, fmt.Errorf("insufficient balance for batch (need %s, have %s)", totalAmount, fromWallet.Balance)
	}

	// 6. Execute each transfer via Wallet Service
	var transactions []Transaction
	var batch *BatchTransaction

	err = s.db.WithTransaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// Create batch record
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

		// Process each transfer
		for i, transfer := range req.Transfers {
			// Validate recipient wallet exists
			toWallet, err := s.getWalletFromService(ctx, transfer.ToWalletID)
			if err != nil {
				return fmt.Errorf("transfer[%d] recipient error: %w", i, err)
			}

			// Validate currency
			if fromWallet.Currency != toWallet.Currency {
				return fmt.Errorf("transfer[%d] currency mismatch", i)
			}

			// Execute individual transfer
			transferReq := WalletTransferRequest{
				FromWalletID:   req.FromWalletID,
				ToWalletID:     transfer.ToWalletID,
				Amount:         transfer.Amount,
				IdempotencyKey: fmt.Sprintf("%s-%d", req.IdempotencyKey, i),
			}

			if err := s.executeWalletTransfer(ctx, &transferReq); err != nil {
				return fmt.Errorf("transfer[%d] execution failed: %w", i, err)
			}

			// Create transaction record
			txn := &Transaction{
				FromWalletID:   req.FromWalletID,
				ToWalletID:     transfer.ToWalletID,
				Amount:         transfer.Amount,
				Currency:       fromWallet.Currency,
				Type:           TypeBatch,
				Status:         StatusCompleted,
				Description:    transfer.Description,
				IdempotencyKey: fmt.Sprintf("%s-%d", req.IdempotencyKey, i),
			}

			createdTxn, err := s.repo.CreateTransactionTx(ctx, tx, txn)
			if err != nil {
				return fmt.Errorf("transfer[%d] record creation failed: %w", i, err)
			}
			transactions = append(transactions, *createdTxn)
		}

		// Update batch status to completed
		if err := s.repo.UpdateBatchTransactionStatusTx(ctx, tx, batch.ID, StatusCompleted); err != nil {
			return fmt.Errorf("failed to update batch status: %w", err)
		}
		batch.Status = StatusCompleted

		// Save outbox event
		event := &outbox.OutboxEvent{
			AggregateID: batch.ID,
			EventType:   "batch.completed",
			Topic:       "transaction.completed",
			Payload: map[string]interface{}{
				"batch_id":       batch.ID,
				"from_wallet_id": req.FromWalletID,
				"total_amount":   totalAmount,
				"count":          len(req.Transfers),
				"completed_at":   time.Now(),
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

	// Set idempotency key
	if err := s.redis.SetIdempotency(ctx, req.IdempotencyKey, 30*time.Minute); err != nil {
		s.logger.Warnf("Failed to set idempotency key: %v", err)
	}

	s.logger.Infof("Batch transfer completed: %s (%d transfers)", batch.ID, len(transactions))
	return batch, transactions, nil
}

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
	fromWallet, err := s.getWalletFromService(ctx, req.FromWalletID)
	if err != nil {
		return nil, fmt.Errorf("source wallet error: %w", err)
	}

	toWallet, err := s.getWalletFromService(ctx, req.ToWalletID)
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
	// 1. Get wallet info
	fromWallet, err := s.getWalletFromService(ctx, txn.FromWalletID)
	if err != nil {
		return fmt.Errorf("source wallet error: %w", err)
	}

	_, err = s.getWalletFromService(ctx, txn.ToWalletID)
	if err != nil {
		return fmt.Errorf("destination wallet error: %w", err)
	}

	// 2. Check balance
	if !hasSufficientBalance(fromWallet.Balance, txn.Amount) {
		return fmt.Errorf("insufficient balance")
	}

	// 3. Execute transfer via Wallet Service
	transferReq := WalletTransferRequest{
		FromWalletID:   txn.FromWalletID,
		ToWalletID:     txn.ToWalletID,
		Amount:         txn.Amount,
		IdempotencyKey: fmt.Sprintf("scheduled-%s", txn.ID),
	}

	if err := s.executeWalletTransfer(ctx, &transferReq); err != nil {
		return fmt.Errorf("wallet transfer failed: %w", err)
	}

	// 4. Update transaction status in DB
	return s.db.WithTransaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// Update transaction status
		if err := s.repo.UpdateTransactionStatusTx(ctx, tx, txn.ID, StatusCompleted); err != nil {
			return err
		}

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