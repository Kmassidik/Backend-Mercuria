package transaction

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	// amountRegex validates monetary amounts (max 4 decimals)
	amountRegex = regexp.MustCompile(`^\d+(\.\d{1,4})?$`)
)

// ValidateAmount validates a monetary amount
// NOTE: Must be positive, numeric, max 4 decimal places
func ValidateAmount(amount string) error {
	amount = strings.TrimSpace(amount)

	if amount == "" {
		return fmt.Errorf("amount is required")
	}

	if !amountRegex.MatchString(amount) {
		return fmt.Errorf("invalid amount format (must be positive number with max 4 decimals)")
	}

	// Check if amount is zero
	if amount == "0" || amount == "0.0" || amount == "0.00" || amount == "0.000" || amount == "0.0000" {
		return fmt.Errorf("amount must be greater than zero")
	}

	return nil
}

// ValidateCreateTransactionRequest validates a P2P transfer request
// NOTE: Ensures basic transfer requirements are met
func ValidateCreateTransactionRequest(req *CreateTransactionRequest) error {
	// Validate wallet IDs
	if strings.TrimSpace(req.FromWalletID) == "" {
		return fmt.Errorf("from_wallet_id is required")
	}

	if strings.TrimSpace(req.ToWalletID) == "" {
		return fmt.Errorf("to_wallet_id is required")
	}

	// Can't transfer to yourself
	if req.FromWalletID == req.ToWalletID {
		return fmt.Errorf("cannot transfer to the same wallet")
	}

	// Validate amount
	if err := ValidateAmount(req.Amount); err != nil {
		return err
	}

	// Validate idempotency key
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return fmt.Errorf("idempotency_key is required")
	}

	return nil
}

// ValidateCreateBatchTransactionRequest validates a batch transfer request
// NOTE: Batch transfers have additional constraints for safety
func ValidateCreateBatchTransactionRequest(req *CreateBatchTransactionRequest) error {
	// Validate source wallet
	if strings.TrimSpace(req.FromWalletID) == "" {
		return fmt.Errorf("from_wallet_id is required")
	}

	// Validate idempotency key
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return fmt.Errorf("idempotency_key is required")
	}

	// Validate transfers list
	if len(req.Transfers) == 0 {
		return fmt.Errorf("transfers list cannot be empty")
	}

	// Limit batch size to prevent abuse
	if len(req.Transfers) > 100 {
		return fmt.Errorf("batch cannot exceed 100 transfers")
	}

	// Track recipients to detect duplicates
	recipients := make(map[string]bool)

	// Validate each transfer item
	for i, transfer := range req.Transfers {
		// Validate recipient wallet ID
		if strings.TrimSpace(transfer.ToWalletID) == "" {
			return fmt.Errorf("transfer[%d]: to_wallet_id is required", i)
		}

		// Can't transfer to self
		if transfer.ToWalletID == req.FromWalletID {
			return fmt.Errorf("transfer[%d]: cannot transfer to source wallet", i)
		}

		// Check for duplicate recipients
		if recipients[transfer.ToWalletID] {
			return fmt.Errorf("transfer[%d]: duplicate recipient %s", i, transfer.ToWalletID)
		}
		recipients[transfer.ToWalletID] = true

		// Validate amount
		if err := ValidateAmount(transfer.Amount); err != nil {
			return fmt.Errorf("transfer[%d]: %w", i, err)
		}
	}

	return nil
}

// ValidateCreateScheduledTransactionRequest validates a scheduled transfer
// NOTE: Scheduled transfers must be in the future with reasonable limits
func ValidateCreateScheduledTransactionRequest(req *CreateScheduledTransactionRequest) error {
	// First validate as a normal transaction
	normalReq := CreateTransactionRequest{
		FromWalletID:   req.FromWalletID,
		ToWalletID:     req.ToWalletID,
		Amount:         req.Amount,
		Description:    req.Description,
		IdempotencyKey: req.IdempotencyKey,
	}

	if err := ValidateCreateTransactionRequest(&normalReq); err != nil {
		return err
	}

	// Validate scheduled time
	if req.ScheduledAt.IsZero() {
		return fmt.Errorf("scheduled_at is required")
	}

	now := time.Now()

	// Must be in the future
	if req.ScheduledAt.Before(now) {
		return fmt.Errorf("scheduled_at must be in the future")
	}

	// Minimum delay: 1 minute (prevents near-instant scheduling issues)
	minScheduledTime := now.Add(1 * time.Minute)
	if req.ScheduledAt.Before(minScheduledTime) {
		return fmt.Errorf("scheduled_at must be at least 1 minute in the future")
	}

	// Maximum delay: 1 year (prevents indefinite pending transfers)
	maxScheduledTime := now.Add(365 * 24 * time.Hour)
	if req.ScheduledAt.After(maxScheduledTime) {
		return fmt.Errorf("scheduled_at cannot be more than 1 year in the future")
	}

	return nil
}

// CalculateBatchTotal calculates the total amount for a batch transfer
// NOTE: Used to verify sender has sufficient balance
func CalculateBatchTotal(transfers []BatchTransferItem) (string, error) {
	// Use big.Float for precise decimal arithmetic
	total := 0.0

	for _, transfer := range transfers {
		// Parse amount
		var amount float64
		_, err := fmt.Sscanf(transfer.Amount, "%f", &amount)
		if err != nil {
			return "", fmt.Errorf("invalid amount in batch: %s", transfer.Amount)
		}

		total += amount
	}

	// Format with 4 decimal places
	return fmt.Sprintf("%.4f", total), nil
}