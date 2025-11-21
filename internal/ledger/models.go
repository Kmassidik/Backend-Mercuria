package ledger

import (
	"time"
)

// LedgerEntry represents a single entry in the double-entry ledger
// NOTE: Immutable audit trail - NEVER update or delete entries
type LedgerEntry struct {
	ID            string                 `json:"id"`
	TransactionID string                 `json:"transaction_id"`    // Links to transaction service
	WalletID      string                 `json:"wallet_id"`         // Which wallet this entry affects
	EntryType     string                 `json:"entry_type"`        // debit or credit
	Amount        string                 `json:"amount"`            // NUMERIC(20,4) for precision
	Currency      string                 `json:"currency"`          // USD, EUR, etc.
	Balance       string                 `json:"balance"`           // Running balance after this entry
	Description   string                 `json:"description"`       // Human-readable description
	Metadata      map[string]interface{} `json:"metadata"`          // Additional context (JSONB)
	CreatedAt     time.Time              `json:"created_at"`
}

// Entry types for double-entry bookkeeping
const (
	EntryTypeDebit  = "debit"   // Money leaving wallet (-)
	EntryTypeCredit = "credit"  // Money entering wallet (+)
)

// TransactionLedger represents all ledger entries for a transaction
// NOTE: Every transaction has 2+ entries (debit + credit)
type TransactionLedger struct {
	TransactionID string         `json:"transaction_id"`
	Entries       []LedgerEntry  `json:"entries"`
	TotalDebits   string         `json:"total_debits"`
	TotalCredits  string         `json:"total_credits"`
	CreatedAt     time.Time      `json:"created_at"`
}

// CreateLedgerEntriesRequest - Internal request for creating entries
// NOTE: This is called by the Kafka consumer, not via HTTP
// ✅ UPDATED: Added balance tracking fields
type CreateLedgerEntriesRequest struct {
	TransactionID     string
	FromWalletID      string
	ToWalletID        string
	Amount            string
	Currency          string
	Description       string
	// ✅ NEW: Optional balance tracking (if provided by transaction service)
	FromBalanceBefore string // Balance before transaction
	FromBalanceAfter  string // Balance after transaction
	ToBalanceBefore   string // Balance before transaction
	ToBalanceAfter    string // Balance after transaction
}

// API Response types

// LedgerEntryResponse - Single entry response
type LedgerEntryResponse struct {
	Entry *LedgerEntry `json:"entry"`
}

// LedgerEntriesResponse - List of entries response
type LedgerEntriesResponse struct {
	Entries []LedgerEntry `json:"entries"`
	Total   int           `json:"total"`
}

// TransactionLedgerResponse - All entries for a transaction
type TransactionLedgerResponse struct {
	TransactionID string        `json:"transaction_id"`
	Entries       []LedgerEntry `json:"entries"`
	Balanced      bool          `json:"balanced"` // debits == credits
}

// WalletLedgerResponse - Ledger history for a wallet
type WalletLedgerResponse struct {
	WalletID      string        `json:"wallet_id"`
	Entries       []LedgerEntry `json:"entries"`
	CurrentBalance string       `json:"current_balance"`
	Total         int           `json:"total"`
}

// ErrorResponse - Standard error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// Kafka event payloads

// TransactionCompletedEvent - Consumed from Kafka
// NOTE: Published by Transaction Service after successful transfer
type TransactionCompletedEvent struct {
	TransactionID string    `json:"transaction_id"`
	FromWalletID  string    `json:"from_wallet_id"`
	ToWalletID    string    `json:"to_wallet_id"`
	Amount        string    `json:"amount"`
	Currency      string    `json:"currency"`
	Type          string    `json:"type"`
	CompletedAt   time.Time `json:"completed_at"`
}

// LedgerEntryCreatedEvent - Published to Kafka
// NOTE: Consumed by Analytics Service for metrics
type LedgerEntryCreatedEvent struct {
	EntryID       string    `json:"entry_id"`
	TransactionID string    `json:"transaction_id"`
	WalletID      string    `json:"wallet_id"`
	EntryType     string    `json:"entry_type"`
	Amount        string    `json:"amount"`
	Currency      string    `json:"currency"`
	Balance       string    `json:"balance"`
	CreatedAt     time.Time `json:"created_at"`
}

// LedgerStats - Statistics for a wallet or period
type LedgerStats struct {
	WalletID     string `json:"wallet_id,omitempty"`
	TotalDebits  string `json:"total_debits"`
	TotalCredits string `json:"total_credits"`
	NetChange    string `json:"net_change"`
	EntryCount   int    `json:"entry_count"`
	FirstEntry   *time.Time `json:"first_entry,omitempty"`
	LastEntry    *time.Time `json:"last_entry,omitempty"`
}