package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kmassidik/mercuria/internal/common/redis"
)

type Service interface {
	// Event Processing
	ProcessLedgerEntryCreated(ctx context.Context, event *LedgerEntryCreatedEvent) error
	ProcessKafkaEvent(ctx context.Context, value []byte) error
	IsEventProcessed(ctx context.Context, eventID string) (bool, error)

	// Metrics Retrieval
	GetDailyMetrics(ctx context.Context, startDate, endDate time.Time) ([]*DailyMetric, error)
	GetHourlyMetrics(ctx context.Context, startTime, endTime time.Time) ([]*HourlyMetric, error)
	GetMetricsSummary(ctx context.Context, startDate, endDate time.Time, period string) (*MetricsSummaryResponse, error)

	// User Analytics
	GetUserAnalytics(ctx context.Context, userID string, startDate, endDate time.Time) (*UserAnalyticsResponse, error)
	GetUserSnapshots(ctx context.Context, userID string, startDate, endDate time.Time) ([]*UserSnapshot, error)
}

type service struct {
	repo  Repository
	redis *redis.Client
}

func NewService(repo Repository, redisClient *redis.Client) Service {
	return &service{
		repo:  repo,
		redis: redisClient,
	}
}

// ProcessKafkaEvent processes incoming Kafka events
func (s *service) ProcessKafkaEvent(ctx context.Context, value []byte) error {
	// Parse the ledger event from Kafka
	var event struct {
		EventID       string                 `json:"event_id"`
		EntryID       string                 `json:"entry_id"`
		TransactionID string                 `json:"transaction_id"`
		WalletID      string                 `json:"wallet_id"`
		EntryType     string                 `json:"entry_type"`
		Amount        string                 `json:"amount"`
		Currency      string                 `json:"currency"`
		Balance       string                 `json:"balance"`
		CreatedAt     string                 `json:"created_at"`
		Metadata      map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := json.Unmarshal(value, &event); err != nil {
		return fmt.Errorf("failed to unmarshal ledger event: %w", err)
	}

	// Convert to LedgerEntryCreatedEvent
	ledgerEvent := &LedgerEntryCreatedEvent{
		EventID:         event.EventID,
		LedgerID:        event.EntryID,
		TransactionID:   event.TransactionID,
		FromWalletID:    event.WalletID,
		ToWalletID:      getToWalletID(event.EntryType, event.Metadata),
		Amount:          parseAmount(event.Amount),
		Fee:             0,
		Status:          TransactionStatusCompleted,
		TransactionType: event.EntryType,
		CreatedAt:       parseTime(event.CreatedAt),
		Metadata:        event.Metadata,
	}

	// Process the event
	return s.ProcessLedgerEntryCreated(ctx, ledgerEvent)
}

// Helper to get opposite wallet from metadata
func getToWalletID(entryType string, metadata map[string]interface{}) string {
	if entryType == "debit" {
		if toWallet, ok := metadata["to_wallet_id"].(string); ok {
			return toWallet
		}
	} else if entryType == "credit" {
		if fromWallet, ok := metadata["from_wallet_id"].(string); ok {
			return fromWallet
		}
	}
	return ""
}

// Helper to parse amount string to float64
func parseAmount(amount string) float64 {
	var result float64
	fmt.Sscanf(amount, "%f", &result)
	return result
}

// Helper to parse time string
func parseTime(timeStr string) time.Time {
	t, _ := time.Parse(time.RFC3339, timeStr)
	return t
}

// ProcessLedgerEntryCreated processes a ledger entry created event
func (s *service) ProcessLedgerEntryCreated(ctx context.Context, event *LedgerEntryCreatedEvent) error {
	startTime := time.Now()

	// Check idempotency - have we processed this event before?
	processed, err := s.IsEventProcessed(ctx, event.EventID)
	if err != nil {
		return fmt.Errorf("failed to check event processing status: %w", err)
	}
	if processed {
		return nil // Already processed, skip
	}

	// Marshal event data for storage
	eventData, err := MarshalEventData(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	// Process the event and update metrics
	if err := s.processEvent(ctx, event); err != nil {
		// Log failed processing
		processingTime := int(time.Since(startTime).Milliseconds())
		errMsg := err.Error()
		logErr := s.logEventProcessing(ctx, event, eventData, processingTime, EventStatusFailed, &errMsg, 0)
		if logErr != nil {
			return fmt.Errorf("failed to log event processing error: %w (original error: %v)", logErr, err)
		}
		return err
	}

	// Log successful processing
	processingTime := int(time.Since(startTime).Milliseconds())
	if err := s.logEventProcessing(ctx, event, eventData, processingTime, EventStatusProcessed, nil, 0); err != nil {
		return fmt.Errorf("failed to log event processing: %w", err)
	}

	// Invalidate Redis cache for affected date
	if err := s.invalidateCacheForDate(ctx, event.CreatedAt); err != nil {
		// Log but don't fail - cache invalidation is not critical
		fmt.Printf("Warning: failed to invalidate cache: %v\n", err)
	}

	return nil
}

// processEvent performs the actual metric aggregation
func (s *service) processEvent(ctx context.Context, event *LedgerEntryCreatedEvent) error {
	txDate := event.CreatedAt.Truncate(24 * time.Hour)
	txHour := event.CreatedAt.Truncate(time.Hour)

	// Determine transaction success
	isSuccess := event.Status == TransactionStatusCompleted
	successCount := int64(0)
	failCount := int64(0)
	if isSuccess {
		successCount = 1
	} else {
		failCount = 1
	}

	// Update daily metrics
	dailyMetric := &DailyMetric{
		MetricDate:             txDate,
		TotalTransactions:      1,
		TotalVolume:            event.Amount,
		TotalFees:              event.Fee,
		UniqueUsers:            1, // Will be calculated properly in aggregation
		SuccessfulTransactions: successCount,
		FailedTransactions:     failCount,
		AvgTransactionValue:    event.Amount,
	}

	if err := s.repo.UpsertDailyMetric(ctx, dailyMetric); err != nil {
		return fmt.Errorf("failed to update daily metric: %w", err)
	}

	// Update hourly metrics
	hourlyMetric := &HourlyMetric{
		MetricHour:             txHour,
		TotalTransactions:      1,
		TotalVolume:            event.Amount,
		TotalFees:              event.Fee,
		UniqueUsers:            1,
		SuccessfulTransactions: successCount,
		FailedTransactions:     failCount,
		AvgTransactionValue:    event.Amount,
		MaxTransactionValue:    event.Amount,
		MinTransactionValue:    event.Amount,
		AvgProcessingTimeMs:    0, // Could be extracted from metadata
	}

	if err := s.repo.UpsertHourlyMetric(ctx, hourlyMetric); err != nil {
		return fmt.Errorf("failed to update hourly metric: %w", err)
	}

	// Update user snapshots for both sender and receiver
	if err := s.updateUserSnapshot(ctx, event, txDate); err != nil {
		return fmt.Errorf("failed to update user snapshots: %w", err)
	}

	return nil
}

// updateUserSnapshot updates snapshots for both sender and receiver
func (s *service) updateUserSnapshot(ctx context.Context, event *LedgerEntryCreatedEvent, snapshotDate time.Time) error {
	// Extract user IDs from wallet IDs (assuming wallet ID contains user info)
	// In a real system, you'd query the wallet service to get user IDs
	// For now, we'll use wallet IDs as proxy for user IDs
	fromUserID := event.FromWalletID
	toUserID := event.ToWalletID

	// Update sender snapshot
	if fromUserID != "" {
		senderSnapshot := &UserSnapshot{
			UserID:            fromUserID,
			SnapshotDate:      snapshotDate,
			TotalSent:         event.Amount,
			TotalReceived:     0,
			TransactionCount:  1,
			SentCount:         1,
			ReceivedCount:     0,
			TotalFeesPaid:     event.Fee,
			LastTransactionAt: &event.CreatedAt,
		}

		if err := s.repo.UpsertUserSnapshot(ctx, senderSnapshot); err != nil {
			return fmt.Errorf("failed to update sender snapshot: %w", err)
		}
	}

	// Update receiver snapshot
	if toUserID != "" {
		receiverSnapshot := &UserSnapshot{
			UserID:            toUserID,
			SnapshotDate:      snapshotDate,
			TotalSent:         0,
			TotalReceived:     event.Amount,
			TransactionCount:  1,
			SentCount:         0,
			ReceivedCount:     1,
			TotalFeesPaid:     0,
			LastTransactionAt: &event.CreatedAt,
		}

		if err := s.repo.UpsertUserSnapshot(ctx, receiverSnapshot); err != nil {
			return fmt.Errorf("failed to update receiver snapshot: %w", err)
		}
	}

	return nil
}

// logEventProcessing creates an event processing log entry
func (s *service) logEventProcessing(ctx context.Context, event *LedgerEntryCreatedEvent, eventData []byte, processingTimeMs int, status string, errorMsg *string, retryCount int) error {
	log := &EventProcessingLog{
		EventID:          event.EventID,
		EventType:        "ledger.entry_created",
		Topic:            "ledger.entry_created",
		Partition:        0, // Will be set by Kafka consumer
		Offset:           0, // Will be set by Kafka consumer
		EventData:        eventData,
		ProcessedAt:      time.Now(),
		ProcessingTimeMs: processingTimeMs,
		Status:           status,
		ErrorMessage:     errorMsg,
		RetryCount:       retryCount,
	}

	return s.repo.CreateEventLog(ctx, log)
}

// IsEventProcessed checks if an event has already been processed (idempotency)
func (s *service) IsEventProcessed(ctx context.Context, eventID string) (bool, error) {
	log, err := s.repo.GetEventLogByEventID(ctx, eventID)
	if err != nil {
		return false, err
	}
	return log != nil, nil
}

// GetDailyMetrics retrieves daily metrics with caching
func (s *service) GetDailyMetrics(ctx context.Context, startDate, endDate time.Time) ([]*DailyMetric, error) {
	// Try cache first
	cacheKey := fmt.Sprintf("analytics:daily:%s:%s", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	cached, err := s.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		var metrics []*DailyMetric
		if err := json.Unmarshal([]byte(cached), &metrics); err == nil {
			return metrics, nil
		}
	}

	// Fetch from database
	metrics, err := s.repo.GetDailyMetrics(ctx, startDate, endDate)
	if err != nil {
		return nil, err
	}

	// Cache the result
	if data, err := json.Marshal(metrics); err == nil {
		s.redis.Set(ctx, cacheKey, data, 24*time.Hour)
	}

	return metrics, nil
}

// GetHourlyMetrics retrieves hourly metrics
func (s *service) GetHourlyMetrics(ctx context.Context, startTime, endTime time.Time) ([]*HourlyMetric, error) {
	return s.repo.GetHourlyMetrics(ctx, startTime, endTime)
}

// GetMetricsSummary retrieves aggregated metrics summary
func (s *service) GetMetricsSummary(ctx context.Context, startDate, endDate time.Time, period string) (*MetricsSummaryResponse, error) {
	// Validate period
	if period != "daily" && period != "hourly" {
		return nil, fmt.Errorf("invalid period: must be 'daily' or 'hourly'")
	}

	// Try cache first
	cacheKey := fmt.Sprintf("analytics:summary:%s:%s:%s", period, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	cached, err := s.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		var summary MetricsSummaryResponse
		if err := json.Unmarshal([]byte(cached), &summary); err == nil {
			return &summary, nil
		}
	}

	// Fetch from database
	summary, err := s.repo.GetMetricsSummary(ctx, startDate, endDate, period)
	if err != nil {
		return nil, err
	}

	// Cache the result
	if data, err := json.Marshal(summary); err == nil {
		s.redis.Set(ctx, cacheKey, data, 1*time.Hour)
	}

	return summary, nil
}

// GetUserAnalytics retrieves user-specific analytics
func (s *service) GetUserAnalytics(ctx context.Context, userID string, startDate, endDate time.Time) (*UserAnalyticsResponse, error) {
	return s.repo.GetUserAnalytics(ctx, userID, startDate, endDate)
}

// GetUserSnapshots retrieves user snapshots
func (s *service) GetUserSnapshots(ctx context.Context, userID string, startDate, endDate time.Time) ([]*UserSnapshot, error) {
	return s.repo.GetUserSnapshots(ctx, userID, startDate, endDate)
}

// invalidateCacheForDate invalidates Redis cache for a specific date
func (s *service) invalidateCacheForDate(ctx context.Context, date time.Time) error {
	dateStr := date.Format("2006-01-02")
	pattern := fmt.Sprintf("analytics:*%s*", dateStr)

	// Scan and delete matching keys
	iter := s.redis.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		if err := s.redis.Del(ctx, iter.Val()).Err(); err != nil {
			return err
		}
	}
	return iter.Err()
}