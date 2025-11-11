package wallet

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	amountRegex   = regexp.MustCompile(`^\d+(\.\d{1,4})?$`)
	currencyRegex = regexp.MustCompile(`^[A-Z]{3}$`)
)

// Supported currencies
var supportedCurrencies = map[string]bool{
	"USD": true,
	"EUR": true,
	"GBP": true,
	"JPY": true,
	"IDR": true,
}

// ValidateCreateWalletRequest validates wallet creation request
func ValidateCreateWalletRequest(req *CreateWalletRequest) error {
	if req.UserID == "" {
		return fmt.Errorf("user_id is required")
	}

	// Normalize currency to uppercase
	req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))

	if req.Currency == "" {
		return fmt.Errorf("currency is required")
	}

	if !currencyRegex.MatchString(req.Currency) {
		return fmt.Errorf("currency must be a 3-letter code")
	}

	if !supportedCurrencies[req.Currency] {
		return fmt.Errorf("currency %s is not supported", req.Currency)
	}

	return nil
}

// ValidateAmount validates a monetary amount
func ValidateAmount(amount string) error {
	amount = strings.TrimSpace(amount)

	if amount == "" {
		return fmt.Errorf("amount is required")
	}

	if !amountRegex.MatchString(amount) {
		return fmt.Errorf("invalid amount format (must be positive number with max 4 decimals)")
	}

	// Check if amount is zero or negative
	if amount == "0" || amount == "0.0" || amount == "0.00" {
		return fmt.Errorf("amount must be greater than zero")
	}

	return nil
}

// ValidateDepositRequest validates deposit request
func ValidateDepositRequest(req *DepositRequest) error {
	if err := ValidateAmount(req.Amount); err != nil {
		return err
	}

	if req.IdempotencyKey == "" {
		return fmt.Errorf("idempotency_key is required")
	}

	return nil
}

// ValidateWithdrawRequest validates withdrawal request
func ValidateWithdrawRequest(req *WithdrawRequest) error {
	if err := ValidateAmount(req.Amount); err != nil {
		return err
	}

	if req.IdempotencyKey == "" {
		return fmt.Errorf("idempotency_key is required")
	}

	return nil
}