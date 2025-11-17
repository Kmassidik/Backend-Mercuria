package wallet

import (
	"time"
)

type Wallet struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Currency  string    `json:"currency"`
	Balance   string    `json:"balance"` 
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type WalletEvent struct {
	ID            string                 `json:"id"`
	WalletID      string                 `json:"wallet_id"`
	EventType     string                 `json:"event_type"`
	Amount        string                 `json:"amount"`
	BalanceBefore string                 `json:"balance_before"`
	BalanceAfter  string                 `json:"balance_after"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
}

type MyWalletsResponse struct {
	Wallets []Wallet `json:"wallets"`  
	Total int `json:"total"`
}


const (
	EventTypeCreated    = "wallet.created"
	EventTypeDeposit    = "wallet.deposit"
	EventTypeWithdrawal = "wallet.withdrawal"
	EventTypeLocked     = "wallet.locked"
	EventTypeUnlocked   = "wallet.unlocked"
)

const (
	StatusActive   = "active"
	StatusLocked   = "locked"
	StatusInactive = "inactive"
)

type CreateWalletRequest struct {
	UserID   string `json:"user_id"`
	Currency string `json:"currency"`
}

type DepositRequest struct {
	Amount            string `json:"amount"`
	IdempotencyKey    string `json:"idempotency_key"`
	Description       string `json:"description,omitempty"`
}

type WithdrawRequest struct {
	Amount            string `json:"amount"`
	IdempotencyKey    string `json:"idempotency_key"`
	Description       string `json:"description,omitempty"`
}

type WalletResponse struct {
	Wallet *Wallet `json:"wallet"`
}

type WalletEventsResponse struct {
	Events []WalletEvent `json:"events"`
	Total  int           `json:"total"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

// Kafka event payloads
type WalletCreatedEvent struct {
	WalletID  string    `json:"wallet_id"`
	UserID    string    `json:"user_id"`
	Currency  string    `json:"currency"`
	CreatedAt time.Time `json:"created_at"`
}

type BalanceUpdatedEvent struct {
	WalletID      string    `json:"wallet_id"`
	UserID        string    `json:"user_id"`
	EventType     string    `json:"event_type"`
	Amount        string    `json:"amount"`
	BalanceBefore string    `json:"balance_before"`
	BalanceAfter  string    `json:"balance_after"`
	Timestamp     time.Time `json:"timestamp"`
}

type TransferRequest struct {
	FromWalletID   string `json:"from_wallet_id"`
	ToWalletID     string `json:"to_wallet_id"`
	Amount         string `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
}