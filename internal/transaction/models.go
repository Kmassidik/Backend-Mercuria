package transaction

import (
	"context"
	"time"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// AuthorizationContextKey is used to pass JWT token between handler and service
	AuthorizationContextKey contextKey = "authorization"
)

// SetAuthorizationInContext adds the Authorization header to context
func SetAuthorizationInContext(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, AuthorizationContextKey, token)
}

// GetAuthorizationFromContext retrieves the Authorization header from context
func GetAuthorizationFromContext(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(AuthorizationContextKey).(string)
	return token, ok
}

// Transaction represents a fund transfer between wallets
// NOTE: This is the core entity for money movement in Mercuria
type Transaction struct {
	ID                string    `json:"id"`
	FromWalletID      string    `json:"from_wallet_id"`       // Source wallet
	ToWalletID        string    `json:"to_wallet_id"`         // Destination wallet
	Amount            string    `json:"amount"`               // Transfer amount (NUMERIC precision)
	Currency          string    `json:"currency"`             // USD, EUR, etc.
	Type              string    `json:"type"`                 // p2p, batch, scheduled
	Status            string    `json:"status"`               // pending, completed, failed, scheduled
	Description       string    `json:"description"`          // Optional memo
	IdempotencyKey    string    `json:"idempotency_key"`      // Prevents duplicate transfers
	ScheduledAt       *time.Time `json:"scheduled_at"`        // For scheduled transfers
	ProcessedAt       *time.Time `json:"processed_at"`        // When transfer completed
	FailureReason     string    `json:"failure_reason"`       // Error message if failed
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// BatchTransaction represents multiple transfers in one request
// NOTE: All transfers in a batch must succeed or all fail (atomic)
type BatchTransaction struct {
	ID             string    `json:"id"`
	FromWalletID   string    `json:"from_wallet_id"`        // Single source wallet
	Transfers      []BatchTransferItem `json:"transfers"`   // List of recipients
	TotalAmount    string    `json:"total_amount"`          // Sum of all transfers
	Currency       string    `json:"currency"`
	Status         string    `json:"status"`
	IdempotencyKey string    `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// BatchTransferItem represents one transfer in a batch
type BatchTransferItem struct {
	ToWalletID  string `json:"to_wallet_id"`
	Amount      string `json:"amount"`
	Description string `json:"description"`
}

// Transaction types
const (
	TypeP2P       = "p2p"        // Simple peer-to-peer transfer
	TypeBatch     = "batch"      // Multiple recipients
	TypeScheduled = "scheduled"  // Future-dated transfer
)

// Transaction statuses
const (
	StatusPending   = "pending"    // Created but not processed
	StatusCompleted = "completed"  // Successfully transferred
	StatusFailed    = "failed"     // Transfer failed
	StatusScheduled = "scheduled"  // Waiting for scheduled time
	StatusCancelled = "cancelled"  // Cancelled by user
)

// CreateTransactionRequest - Simple P2P transfer request
// NOTE: This is the most common transaction type
type CreateTransactionRequest struct {
	FromWalletID   string `json:"from_wallet_id"`
	ToWalletID     string `json:"to_wallet_id"`
	Amount         string `json:"amount"`
	Description    string `json:"description"`
	IdempotencyKey string `json:"idempotency_key"`
}

// CreateBatchTransactionRequest - Multiple recipients in one request
// NOTE: Useful for payroll, bulk payments, etc.
type CreateBatchTransactionRequest struct {
	FromWalletID   string              `json:"from_wallet_id"`
	Transfers      []BatchTransferItem `json:"transfers"`
	IdempotencyKey string              `json:"idempotency_key"`
}

// CreateScheduledTransactionRequest - Future-dated transfer
// NOTE: Transfer executes automatically at scheduled time
type CreateScheduledTransactionRequest struct {
	FromWalletID   string    `json:"from_wallet_id"`
	ToWalletID     string    `json:"to_wallet_id"`
	Amount         string    `json:"amount"`
	Description    string    `json:"description"`
	ScheduledAt    time.Time `json:"scheduled_at"`        // When to execute
	IdempotencyKey string    `json:"idempotency_key"`
}

// TransactionResponse - API response wrapper
type TransactionResponse struct {
	Transaction *Transaction `json:"transaction"`
}

// BatchTransactionResponse - API response for batch transfers
type BatchTransactionResponse struct {
	BatchTransaction *BatchTransaction `json:"batch_transaction"`
	Transactions     []Transaction     `json:"transactions"` // Individual transfers
}

// TransactionListResponse - List of transactions with pagination
type TransactionListResponse struct {
	Transactions []Transaction `json:"transactions"`
	Total        int           `json:"total"`
	Limit        int           `json:"limit"`
	Offset       int           `json:"offset"`
}

// ErrorResponse - Standard error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// Kafka event payloads
// NOTE: These events are published to Kafka after successful transfers

// TransactionCompletedEvent - Published when transfer completes
type TransactionCompletedEvent struct {
	TransactionID  string    `json:"transaction_id"`
	FromWalletID   string    `json:"from_wallet_id"`
	ToWalletID     string    `json:"to_wallet_id"`
	Amount         string    `json:"amount"`
	Currency       string    `json:"currency"`
	Type           string    `json:"type"`
	CompletedAt    time.Time `json:"completed_at"`
}

// TransactionFailedEvent - Published when transfer fails
type TransactionFailedEvent struct {
	TransactionID string    `json:"transaction_id"`
	FromWalletID  string    `json:"from_wallet_id"`
	ToWalletID    string    `json:"to_wallet_id"`
	Amount        string    `json:"amount"`
	Reason        string    `json:"reason"`
	FailedAt      time.Time `json:"failed_at"`
}

// BatchTransactionCompletedEvent - Published when batch completes
type BatchTransactionCompletedEvent struct {
	BatchID      string    `json:"batch_id"`
	FromWalletID string    `json:"from_wallet_id"`
	TotalAmount  string    `json:"total_amount"`
	Count        int       `json:"count"`        // Number of transfers
	CompletedAt  time.Time `json:"completed_at"`
}