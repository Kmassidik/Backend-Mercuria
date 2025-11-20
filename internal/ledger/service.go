package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/kmassidik/mercuria/internal/common/db"
	"github.com/kmassidik/mercuria/internal/common/logger"
	"github.com/kmassidik/mercuria/pkg/outbox"
)

type Service struct {
	repo       *Repository
	outboxRepo *outbox.Repository
	db         *db.DB
	logger     *logger.Logger
}

func NewService(
	repo *Repository,
	outboxRepo *outbox.Repository,
	database *db.DB,
	log *logger.Logger,
) *Service {
	return &Service{
		repo:       repo,
		outboxRepo: outboxRepo,
		db:         database,
		logger:     log,
	}
}

// CreateLedgerEntries creates double-entry ledger entries for a transaction
func (s *Service) CreateLedgerEntries(ctx context.Context, req *CreateLedgerEntriesRequest) ([]LedgerEntry, error) {
    var entries []LedgerEntry

    // Check if ledger entries already exist for this transaction (idempotency)
    existingEntries, err := s.repo.GetEntriesByTransaction(ctx, req.TransactionID)
    if err == nil && len(existingEntries) > 0 {
        s.logger.Infof("Ledger entries already exist for transaction %s, skipping", req.TransactionID)
        return existingEntries, nil
    }

    // Execute in transaction to ensure atomicity
    err = s.db.WithTransaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
        // Get current balances
        fromBalance, err := s.repo.GetLatestBalance(ctx, req.FromWalletID)
        if err != nil {
            return fmt.Errorf("failed to get from balance: %w", err)
        }

        toBalance, err := s.repo.GetLatestBalance(ctx, req.ToWalletID)
        if err != nil {
            return fmt.Errorf("failed to get to balance: %w", err)
        }

        // Calculate new balances
        newFromBalance, err := subtractAmounts(fromBalance, req.Amount)
        if err != nil {
            return fmt.Errorf("failed to calculate from balance: %w", err)
        }

        newToBalance, err := addAmounts(toBalance, req.Amount)
        if err != nil {
            return fmt.Errorf("failed to calculate to balance: %w", err)
        }

        // Create DEBIT entry
        debitEntry := &LedgerEntry{
            TransactionID: req.TransactionID,
            WalletID:      req.FromWalletID,
            EntryType:     EntryTypeDebit,
            Amount:        req.Amount,
            Currency:      req.Currency,
            Balance:       newFromBalance,
            Description:   fmt.Sprintf("Transfer to %s: %s", req.ToWalletID, req.Description),
            Metadata: map[string]interface{}{
                "to_wallet_id": req.ToWalletID,
            },
        }

        createdDebit, err := s.repo.CreateLedgerEntryTx(ctx, tx, debitEntry)
        if err != nil {
            return fmt.Errorf("failed to create debit entry: %w", err)
        }
        entries = append(entries, *createdDebit)

        // Create CREDIT entry
        creditEntry := &LedgerEntry{
            TransactionID: req.TransactionID,
            WalletID:      req.ToWalletID,
            EntryType:     EntryTypeCredit,
            Amount:        req.Amount,
            Currency:      req.Currency,
            Balance:       newToBalance,
            Description:   fmt.Sprintf("Transfer from %s: %s", req.FromWalletID, req.Description),
            Metadata: map[string]interface{}{
                "from_wallet_id": req.FromWalletID,
            },
        }

        createdCredit, err := s.repo.CreateLedgerEntryTx(ctx, tx, creditEntry)
        if err != nil {
            return fmt.Errorf("failed to create credit entry: %w", err)
        }
        entries = append(entries, *createdCredit)

        // Create outbox events
        for _, entry := range entries {
            event := &outbox.OutboxEvent{
                AggregateID: entry.ID,
                EventType:   "ledger.entry_created",
                Topic:       "ledger.entry_created",
                Payload: map[string]interface{}{
                    "entry_id":       entry.ID,
                    "transaction_id": entry.TransactionID,
                    "wallet_id":      entry.WalletID,
                    "entry_type":     entry.EntryType,
                    "amount":         entry.Amount,
                    "currency":       entry.Currency,
                    "balance":        entry.Balance,
                    "created_at":     entry.CreatedAt,
                },
            }

            if err := s.outboxRepo.SaveEvent(ctx, tx, event); err != nil {
                return fmt.Errorf("failed to save outbox event: %w", err)
            }
        }

        return nil
    })

    if err != nil {
        s.logger.Errorf("Failed to create ledger entries: %v", err)
        return nil, err
    }

    // Verify double-entry balance
    balanced, err := s.repo.VerifyTransactionBalance(ctx, req.TransactionID)
    if err != nil {
        s.logger.Errorf("Failed to verify transaction balance: %v", err)
    } else if !balanced {
        s.logger.Errorf("⚠️  CRITICAL: Transaction %s is unbalanced!", req.TransactionID)
    }

    s.logger.Infof("✅ Ledger entries created for transaction %s: %d entries", req.TransactionID, len(entries))
    return entries, nil
}
// GetLedgerEntry retrieves a single ledger entry
func (s *Service) GetLedgerEntry(ctx context.Context, id string) (*LedgerEntry, error) {
	return s.repo.GetLedgerEntry(ctx, id)
}

// GetTransactionLedger retrieves all ledger entries for a transaction
func (s *Service) GetTransactionLedger(ctx context.Context, transactionID string) (*TransactionLedger, error) {
	entries, err := s.repo.GetEntriesByTransaction(ctx, transactionID)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no ledger entries found for transaction")
	}

	// Calculate totals
	totalDebits := "0.0000"
	totalCredits := "0.0000"

	for _, entry := range entries {
		if entry.EntryType == EntryTypeDebit {
			totalDebits, _ = addAmounts(totalDebits, entry.Amount)
		} else {
			totalCredits, _ = addAmounts(totalCredits, entry.Amount)
		}
	}

	return &TransactionLedger{
		TransactionID: transactionID,
		Entries:       entries,
		TotalDebits:   totalDebits,
		TotalCredits:  totalCredits,
		CreatedAt:     entries[0].CreatedAt,
	}, nil
}

// GetWalletLedger retrieves ledger history for a wallet
func (s *Service) GetWalletLedger(ctx context.Context, walletID string, limit, offset int) ([]LedgerEntry, string, error) {
	entries, err := s.repo.GetEntriesByWallet(ctx, walletID, limit, offset)
	if err != nil {
		return nil, "", err
	}

	// Get current balance
	balance, err := s.repo.GetLatestBalance(ctx, walletID)
	if err != nil {
		return nil, "", err
	}

	return entries, balance, nil
}

// GetWalletStats retrieves statistics for a wallet
func (s *Service) GetWalletStats(ctx context.Context, walletID string) (*LedgerStats, error) {
	return s.repo.GetWalletStats(ctx, walletID)
}

// GetAllEntries retrieves all ledger entries (admin/audit)
func (s *Service) GetAllEntries(ctx context.Context, limit, offset int) ([]LedgerEntry, error) {
	return s.repo.GetAllEntriesPaginated(ctx, limit, offset)
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
	
	// Check for negative result
	if result.Sign() < 0 {
		return "", fmt.Errorf("result would be negative")
	}
	
	return result.Text('f', 4), nil
}

// ProcessTransactionEvent handles incoming transaction events from Kafka
func (s *Service) ProcessTransactionEvent(ctx context.Context, key, value []byte) error {
	s.logger.Debugf("Processing Kafka transaction event, key=%s", string(key))

	// Parse event - match actual Kafka payload structure
	var event struct {
		TransactionID string `json:"transaction_id"`
		FromWalletID  string `json:"from_wallet_id"`
		ToWalletID    string `json:"to_wallet_id"`
		Amount        string `json:"amount"`
		Currency      string `json:"currency"`
		Type          string `json:"type"`
	}

	if err := json.Unmarshal(value, &event); err != nil {
		s.logger.Errorf("Failed to unmarshal transaction event: %v", err)
		return err
	}

	// Validate required fields
	if event.TransactionID == "" {
		return fmt.Errorf("missing transaction_id in event")
	}
	if event.FromWalletID == "" {
		return fmt.Errorf("missing from_wallet_id in event")
	}
	if event.ToWalletID == "" {
		return fmt.Errorf("missing to_wallet_id in event")
	}
	if event.Amount == "" {
		return fmt.Errorf("missing amount in event")
	}

	s.logger.Infof("Processing transaction: %s (type=%s, amount=%s %s)", 
		event.TransactionID, event.Type, event.Amount, event.Currency)

	// Create ledger entries request
	req := &CreateLedgerEntriesRequest{
		TransactionID: event.TransactionID,
		FromWalletID:  event.FromWalletID,
		ToWalletID:    event.ToWalletID,
		Amount:        event.Amount,
		Currency:      event.Currency,
		Description:   fmt.Sprintf("%s transfer", event.Type),
	}

	// Create double-entry ledger records
	entries, err := s.CreateLedgerEntries(ctx, req)
	if err != nil {
		s.logger.Errorf("Failed to create ledger entries for transaction %s: %v", 
			event.TransactionID, err)
		return err
	}

	s.logger.Infof("✅ Created %d ledger entries for transaction %s", 
		len(entries), event.TransactionID)
	return nil
}
