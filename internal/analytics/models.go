package analytics

import (
	"time"
)

// DailyMetric represents aggregated daily statistics
type DailyMetric struct {
	ID                     int64     `json:"id" db:"id"`
	MetricDate             time.Time `json:"metric_date" db:"metric_date"`
	TotalTransactions      int64     `json:"total_transactions" db:"total_transactions"`
	TotalVolume            float64   `json:"total_volume" db:"total_volume"`
	TotalFees              float64   `json:"total_fees" db:"total_fees"`
	UniqueUsers            int64     `json:"unique_users" db:"unique_users"`
	SuccessfulTransactions int64     `json:"successful_transactions" db:"successful_transactions"`
	FailedTransactions     int64     `json:"failed_transactions" db:"failed_transactions"`
	AvgTransactionValue    float64   `json:"avg_transaction_value" db:"avg_transaction_value"`
	CreatedAt              time.Time `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time `json:"updated_at" db:"updated_at"`
}

// HourlyMetric represents aggregated hourly statistics for real-time monitoring
type HourlyMetric struct {
	ID                     int64     `json:"id" db:"id"`
	MetricHour             time.Time `json:"metric_hour" db:"metric_hour"`
	TotalTransactions      int64     `json:"total_transactions" db:"total_transactions"`
	TotalVolume            float64   `json:"total_volume" db:"total_volume"`
	TotalFees              float64   `json:"total_fees" db:"total_fees"`
	UniqueUsers            int64     `json:"unique_users" db:"unique_users"`
	SuccessfulTransactions int64     `json:"successful_transactions" db:"successful_transactions"`
	FailedTransactions     int64     `json:"failed_transactions" db:"failed_transactions"`
	AvgTransactionValue    float64   `json:"avg_transaction_value" db:"avg_transaction_value"`
	MaxTransactionValue    float64   `json:"max_transaction_value" db:"max_transaction_value"`
	MinTransactionValue    float64   `json:"min_transaction_value" db:"min_transaction_value"`
	AvgProcessingTimeMs    int       `json:"avg_processing_time_ms" db:"avg_processing_time_ms"`
	CreatedAt              time.Time `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time `json:"updated_at" db:"updated_at"`
}

// UserSnapshot represents per-user aggregated statistics
type UserSnapshot struct {
	ID                 int64      `json:"id" db:"id"`
	UserID             string     `json:"user_id" db:"user_id"`
	SnapshotDate       time.Time  `json:"snapshot_date" db:"snapshot_date"`
	TotalSent          float64    `json:"total_sent" db:"total_sent"`
	TotalReceived      float64    `json:"total_received" db:"total_received"`
	TransactionCount   int64      `json:"transaction_count" db:"transaction_count"`
	SentCount          int64      `json:"sent_count" db:"sent_count"`
	ReceivedCount      int64      `json:"received_count" db:"received_count"`
	TotalFeesPaid      float64    `json:"total_fees_paid" db:"total_fees_paid"`
	LastTransactionAt  *time.Time `json:"last_transaction_at,omitempty" db:"last_transaction_at"`
	CreatedAt          time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at" db:"updated_at"`
}

// EventProcessingLog tracks processed Kafka events for idempotency
type EventProcessingLog struct {
	ID                int64     `json:"id" db:"id"`
	EventID           string    `json:"event_id" db:"event_id"`
	EventType         string    `json:"event_type" db:"event_type"`
	Topic             string    `json:"topic" db:"topic"`
	Partition         int       `json:"partition" db:"partition"`
	Offset            int64     `json:"offset" db:"offset"`
	EventData         []byte    `json:"event_data,omitempty" db:"event_data"`
	ProcessedAt       time.Time `json:"processed_at" db:"processed_at"`
	ProcessingTimeMs  int       `json:"processing_time_ms" db:"processing_time_ms"`
	Status            string    `json:"status" db:"status"`
	ErrorMessage      *string   `json:"error_message,omitempty" db:"error_message"`
	RetryCount        int       `json:"retry_count" db:"retry_count"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
}

// LedgerEntryCreatedEvent represents the Kafka event from ledger service
type LedgerEntryCreatedEvent struct {
	EventID         string    `json:"event_id"`
	LedgerID        string    `json:"ledger_id"`
	TransactionID   string    `json:"transaction_id"`
	FromWalletID    string    `json:"from_wallet_id"`
	ToWalletID      string    `json:"to_wallet_id"`
	Amount          float64   `json:"amount"`
	Fee             float64   `json:"fee"`
	Status          string    `json:"status"`
	TransactionType string    `json:"transaction_type"`
	CreatedAt       time.Time `json:"created_at"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// MetricsSummaryResponse represents API response for metrics summary
type MetricsSummaryResponse struct {
	Period             string  `json:"period"` // "daily", "hourly"
	TotalTransactions  int64   `json:"total_transactions"`
	TotalVolume        float64 `json:"total_volume"`
	TotalFees          float64 `json:"total_fees"`
	UniqueUsers        int64   `json:"unique_users"`
	SuccessRate        float64 `json:"success_rate"`
	AvgTransactionSize float64 `json:"avg_transaction_size"`
}

// UserAnalyticsResponse represents API response for user analytics
type UserAnalyticsResponse struct {
	UserID            string     `json:"user_id"`
	Period            string     `json:"period"`
	TotalSent         float64    `json:"total_sent"`
	TotalReceived     float64    `json:"total_received"`
	NetAmount         float64    `json:"net_amount"`
	TransactionCount  int64      `json:"transaction_count"`
	TotalFeesPaid     float64    `json:"total_fees_paid"`
	LastTransactionAt *time.Time `json:"last_transaction_at,omitempty"`
}

// GetMetricsRequest represents request parameters for metrics API
type GetMetricsRequest struct {
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
	Period    string    `json:"period"` // "daily" or "hourly"
}

// GetUserAnalyticsRequest represents request parameters for user analytics API
type GetUserAnalyticsRequest struct {
	UserID    string    `json:"user_id"`
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
}

// EventProcessingStatus constants
const (
	EventStatusProcessed = "processed"
	EventStatusFailed    = "failed"
	EventStatusRetrying  = "retrying"
)

// Transaction status constants
const (
	TransactionStatusCompleted = "completed"
	TransactionStatusFailed    = "failed"
	TransactionStatusPending   = "pending"
)