package analytics

import (
	"fmt"
	"time"
)

// ValidateDateRange validates start and end dates
func ValidateDateRange(startDate, endDate time.Time, maxDays int) error {
	if endDate.Before(startDate) {
		return fmt.Errorf("end_date must be after start_date")
	}

	duration := endDate.Sub(startDate)
	maxDuration := time.Duration(maxDays) * 24 * time.Hour

	if duration > maxDuration {
		return fmt.Errorf("date range cannot exceed %d days", maxDays)
	}

	return nil
}

// ValidatePeriod validates the period parameter
func ValidatePeriod(period string) error {
	if period != "daily" && period != "hourly" {
		return fmt.Errorf("invalid period: must be 'daily' or 'hourly'")
	}
	return nil
}

// ValidateUserID validates user ID format
func ValidateUserID(userID string) error {
	if userID == "" {
		return fmt.Errorf("user_id is required")
	}

	if len(userID) < 3 || len(userID) > 36 {
		return fmt.Errorf("invalid user_id length: must be between 3 and 36 characters")
	}

	return nil
}

// ValidateLedgerEvent validates a ledger entry created event
func ValidateLedgerEvent(event *LedgerEntryCreatedEvent) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}

	if event.EventID == "" {
		return fmt.Errorf("event_id is required")
	}

	if event.TransactionID == "" {
		return fmt.Errorf("transaction_id is required")
	}

	if event.Amount < 0 {
		return fmt.Errorf("amount must be non-negative")
	}

	if event.Fee < 0 {
		return fmt.Errorf("fee must be non-negative")
	}

	if event.Status == "" {
		return fmt.Errorf("status is required")
	}

	if event.CreatedAt.IsZero() {
		return fmt.Errorf("created_at is required")
	}

	return nil
}

// ValidateDailyMetric validates a daily metric
func ValidateDailyMetric(metric *DailyMetric) error {
	if metric == nil {
		return fmt.Errorf("metric is nil")
	}

	if metric.MetricDate.IsZero() {
		return fmt.Errorf("metric_date is required")
	}

	if metric.TotalTransactions < 0 {
		return fmt.Errorf("total_transactions cannot be negative")
	}

	if metric.TotalVolume < 0 {
		return fmt.Errorf("total_volume cannot be negative")
	}

	if metric.TotalFees < 0 {
		return fmt.Errorf("total_fees cannot be negative")
	}

	if metric.UniqueUsers < 0 {
		return fmt.Errorf("unique_users cannot be negative")
	}

	return nil
}

// ValidateHourlyMetric validates an hourly metric
func ValidateHourlyMetric(metric *HourlyMetric) error {
	if metric == nil {
		return fmt.Errorf("metric is nil")
	}

	if metric.MetricHour.IsZero() {
		return fmt.Errorf("metric_hour is required")
	}

	if metric.TotalTransactions < 0 {
		return fmt.Errorf("total_transactions cannot be negative")
	}

	if metric.TotalVolume < 0 {
		return fmt.Errorf("total_volume cannot be negative")
	}

	if metric.MaxTransactionValue < 0 {
		return fmt.Errorf("max_transaction_value cannot be negative")
	}

	if metric.MinTransactionValue < 0 {
		return fmt.Errorf("min_transaction_value cannot be negative")
	}

	if metric.MaxTransactionValue < metric.MinTransactionValue {
		return fmt.Errorf("max_transaction_value must be >= min_transaction_value")
	}

	return nil
}

// ValidateUserSnapshot validates a user snapshot
func ValidateUserSnapshot(snapshot *UserSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot is nil")
	}

	if err := ValidateUserID(snapshot.UserID); err != nil {
		return err
	}

	if snapshot.SnapshotDate.IsZero() {
		return fmt.Errorf("snapshot_date is required")
	}

	if snapshot.TotalSent < 0 {
		return fmt.Errorf("total_sent cannot be negative")
	}

	if snapshot.TotalReceived < 0 {
		return fmt.Errorf("total_received cannot be negative")
	}

	if snapshot.TransactionCount < 0 {
		return fmt.Errorf("transaction_count cannot be negative")
	}

	return nil
}