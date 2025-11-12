package transaction

import (
	"testing"
	"time"
)

// TEST: Validate simple P2P transfer request
// NOTE: This defines what a valid transfer looks like
func TestValidateCreateTransactionRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateTransactionRequest
		wantErr bool
	}{
		{
			name: "valid P2P transfer",
			req: CreateTransactionRequest{
				FromWalletID:   "wallet-123",
				ToWalletID:     "wallet-456",
				Amount:         "100.50",
				Description:    "Payment for services",
				IdempotencyKey: "txn-key-1",
			},
			wantErr: false,
		},
		{
			name: "empty from wallet",
			req: CreateTransactionRequest{
				FromWalletID:   "",
				ToWalletID:     "wallet-456",
				Amount:         "100.50",
				IdempotencyKey: "txn-key-2",
			},
			wantErr: true,
		},
		{
			name: "empty to wallet",
			req: CreateTransactionRequest{
				FromWalletID:   "wallet-123",
				ToWalletID:     "",
				Amount:         "100.50",
				IdempotencyKey: "txn-key-3",
			},
			wantErr: true,
		},
		{
			name: "same wallet (can't transfer to self)",
			req: CreateTransactionRequest{
				FromWalletID:   "wallet-123",
				ToWalletID:     "wallet-123",
				Amount:         "100.50",
				IdempotencyKey: "txn-key-4",
			},
			wantErr: true,
		},
		{
			name: "invalid amount",
			req: CreateTransactionRequest{
				FromWalletID:   "wallet-123",
				ToWalletID:     "wallet-456",
				Amount:         "invalid",
				IdempotencyKey: "txn-key-5",
			},
			wantErr: true,
		},
		{
			name: "negative amount",
			req: CreateTransactionRequest{
				FromWalletID:   "wallet-123",
				ToWalletID:     "wallet-456",
				Amount:         "-50.00",
				IdempotencyKey: "txn-key-6",
			},
			wantErr: true,
		},
		{
			name: "zero amount",
			req: CreateTransactionRequest{
				FromWalletID:   "wallet-123",
				ToWalletID:     "wallet-456",
				Amount:         "0",
				IdempotencyKey: "txn-key-7",
			},
			wantErr: true,
		},
		{
			name: "missing idempotency key",
			req: CreateTransactionRequest{
				FromWalletID: "wallet-123",
				ToWalletID:   "wallet-456",
				Amount:       "100.50",
				IdempotencyKey: "",
			},
			wantErr: true,
		},
		{
			name: "too many decimals (max 4)",
			req: CreateTransactionRequest{
				FromWalletID:   "wallet-123",
				ToWalletID:     "wallet-456",
				Amount:         "100.12345",
				IdempotencyKey: "txn-key-8",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateTransactionRequest(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreateTransactionRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TEST: Validate batch transfer request
// NOTE: Batch transfers have additional validation rules
func TestValidateCreateBatchTransactionRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateBatchTransactionRequest
		wantErr bool
	}{
		{
			name: "valid batch transfer",
			req: CreateBatchTransactionRequest{
				FromWalletID: "wallet-123",
				Transfers: []BatchTransferItem{
					{ToWalletID: "wallet-456", Amount: "50.00", Description: "Payment 1"},
					{ToWalletID: "wallet-789", Amount: "75.50", Description: "Payment 2"},
				},
				IdempotencyKey: "batch-key-1",
			},
			wantErr: false,
		},
		{
			name: "empty transfers list",
			req: CreateBatchTransactionRequest{
				FromWalletID:   "wallet-123",
				Transfers:      []BatchTransferItem{},
				IdempotencyKey: "batch-key-2",
			},
			wantErr: true,
		},
		{
			name: "too many transfers (max 100)",
			req: CreateBatchTransactionRequest{
				FromWalletID:   "wallet-123",
				Transfers:      make([]BatchTransferItem, 101),
				IdempotencyKey: "batch-key-3",
			},
			wantErr: true,
		},
		{
			name: "duplicate recipient in batch",
			req: CreateBatchTransactionRequest{
				FromWalletID: "wallet-123",
				Transfers: []BatchTransferItem{
					{ToWalletID: "wallet-456", Amount: "50.00"},
					{ToWalletID: "wallet-456", Amount: "25.00"}, // Duplicate!
				},
				IdempotencyKey: "batch-key-4",
			},
			wantErr: true,
		},
		{
			name: "transfer to self in batch",
			req: CreateBatchTransactionRequest{
				FromWalletID: "wallet-123",
				Transfers: []BatchTransferItem{
					{ToWalletID: "wallet-456", Amount: "50.00"},
					{ToWalletID: "wallet-123", Amount: "25.00"}, // To self!
				},
				IdempotencyKey: "batch-key-5",
			},
			wantErr: true,
		},
		{
			name: "invalid amount in batch item",
			req: CreateBatchTransactionRequest{
				FromWalletID: "wallet-123",
				Transfers: []BatchTransferItem{
					{ToWalletID: "wallet-456", Amount: "50.00"},
					{ToWalletID: "wallet-789", Amount: "-10.00"}, // Negative!
				},
				IdempotencyKey: "batch-key-6",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateBatchTransactionRequest(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreateBatchTransactionRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TEST: Validate scheduled transfer request
// NOTE: Scheduled transfers must be in the future
func TestValidateCreateScheduledTransactionRequest(t *testing.T) {
	now := time.Now()
	
	tests := []struct {
		name    string
		req     CreateScheduledTransactionRequest
		wantErr bool
	}{
		{
			name: "valid scheduled transfer",
			req: CreateScheduledTransactionRequest{
				FromWalletID:   "wallet-123",
				ToWalletID:     "wallet-456",
				Amount:         "100.00",
				Description:    "Future payment",
				ScheduledAt:    now.Add(24 * time.Hour), // Tomorrow
				IdempotencyKey: "scheduled-key-1",
			},
			wantErr: false,
		},
		{
			name: "scheduled in the past",
			req: CreateScheduledTransactionRequest{
				FromWalletID:   "wallet-123",
				ToWalletID:     "wallet-456",
				Amount:         "100.00",
				ScheduledAt:    now.Add(-1 * time.Hour), // Past!
				IdempotencyKey: "scheduled-key-2",
			},
			wantErr: true,
		},
		{
			name: "scheduled too far in future (max 1 year)",
			req: CreateScheduledTransactionRequest{
				FromWalletID:   "wallet-123",
				ToWalletID:     "wallet-456",
				Amount:         "100.00",
				ScheduledAt:    now.Add(366 * 24 * time.Hour), // > 1 year
				IdempotencyKey: "scheduled-key-3",
			},
			wantErr: true,
		},
		{
			name: "scheduled less than 1 minute in future (min delay)",
			req: CreateScheduledTransactionRequest{
				FromWalletID:   "wallet-123",
				ToWalletID:     "wallet-456",
				Amount:         "100.00",
				ScheduledAt:    now.Add(30 * time.Second), // Too soon!
				IdempotencyKey: "scheduled-key-4",
			},
			wantErr: true,
		},
		{
			name: "zero scheduled time",
			req: CreateScheduledTransactionRequest{
				FromWalletID:   "wallet-123",
				ToWalletID:     "wallet-456",
				Amount:         "100.00",
				ScheduledAt:    time.Time{}, // Zero value
				IdempotencyKey: "scheduled-key-5",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateScheduledTransactionRequest(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreateScheduledTransactionRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TEST: Validate transaction amount (shared validation)
// NOTE: Used by all transaction types
func TestValidateAmount(t *testing.T) {
	tests := []struct {
		name    string
		amount  string
		wantErr bool
	}{
		{
			name:    "valid amount",
			amount:  "100.50",
			wantErr: false,
		},
		{
			name:    "valid integer",
			amount:  "100",
			wantErr: false,
		},
		{
			name:    "valid with 4 decimals",
			amount:  "100.1234",
			wantErr: false,
		},
		{
			name:    "zero amount",
			amount:  "0",
			wantErr: true,
		},
		{
			name:    "negative amount",
			amount:  "-50.00",
			wantErr: true,
		},
		{
			name:    "invalid format",
			amount:  "abc",
			wantErr: true,
		},
		{
			name:    "too many decimals",
			amount:  "100.12345",
			wantErr: true,
		},
		{
			name:    "empty amount",
			amount:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAmount(tt.amount)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAmount() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}