package wallet

import (
	"testing"
)

func TestValidateCreateWalletRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateWalletRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: CreateWalletRequest{
				UserID:   "user-123",
				Currency: "USD",
			},
			wantErr: false,
		},
		{
			name: "valid with lowercase currency",
			req: CreateWalletRequest{
				UserID:   "user-123",
				Currency: "eur",
			},
			wantErr: false,
		},
		{
			name: "empty user_id",
			req: CreateWalletRequest{
				UserID:   "",
				Currency: "USD",
			},
			wantErr: true,
		},
		{
			name: "invalid currency code",
			req: CreateWalletRequest{
				UserID:   "user-123",
				Currency: "INVALID",
			},
			wantErr: true,
		},
		{
			name: "empty currency",
			req: CreateWalletRequest{
				UserID:   "user-123",
				Currency: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateWalletRequest(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreateWalletRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

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
			name:    "zero is valid",
			amount:  "0",
			wantErr: false,
		},
		{
			name:    "negative amount",
			amount:  "-100.50",
			wantErr: true,
		},
		{
			name:    "empty amount",
			amount:  "",
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

func TestValidateDepositRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     DepositRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: DepositRequest{
				Amount:         "100.50",
				IdempotencyKey: "key-123",
			},
			wantErr: false,
		},
		{
			name: "invalid amount",
			req: DepositRequest{
				Amount:         "-100",
				IdempotencyKey: "key-123",
			},
			wantErr: true,
		},
		{
			name: "missing idempotency key",
			req: DepositRequest{
				Amount:         "100.50",
				IdempotencyKey: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDepositRequest(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDepositRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateWithdrawRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     WithdrawRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: WithdrawRequest{
				Amount:         "50.25",
				IdempotencyKey: "key-456",
			},
			wantErr: false,
		},
		{
			name: "invalid amount",
			req: WithdrawRequest{
				Amount:         "invalid",
				IdempotencyKey: "key-456",
			},
			wantErr: true,
		},
		{
			name: "missing idempotency key",
			req: WithdrawRequest{
				Amount:         "50.25",
				IdempotencyKey: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWithdrawRequest(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWithdrawRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}