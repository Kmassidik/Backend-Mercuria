package analytics

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type Repository interface {
	// Daily Metrics
	UpsertDailyMetric(ctx context.Context, metric *DailyMetric) error
	GetDailyMetrics(ctx context.Context, startDate, endDate time.Time) ([]*DailyMetric, error)
	GetDailyMetricByDate(ctx context.Context, date time.Time) (*DailyMetric, error)

	// Hourly Metrics
	UpsertHourlyMetric(ctx context.Context, metric *HourlyMetric) error
	GetHourlyMetrics(ctx context.Context, startTime, endTime time.Time) ([]*HourlyMetric, error)
	GetHourlyMetricByHour(ctx context.Context, hour time.Time) (*HourlyMetric, error)

	// User Snapshots
	UpsertUserSnapshot(ctx context.Context, snapshot *UserSnapshot) error
	GetUserSnapshots(ctx context.Context, userID string, startDate, endDate time.Time) ([]*UserSnapshot, error)
	GetUserSnapshotByDate(ctx context.Context, userID string, date time.Time) (*UserSnapshot, error)

	// Event Processing Log
	CreateEventLog(ctx context.Context, log *EventProcessingLog) error
	GetEventLogByEventID(ctx context.Context, eventID string) (*EventProcessingLog, error)
	UpdateEventLogStatus(ctx context.Context, eventID, status string, errorMsg *string, retryCount int) error

	// Analytics Queries
	GetMetricsSummary(ctx context.Context, startDate, endDate time.Time, period string) (*MetricsSummaryResponse, error)
	GetUserAnalytics(ctx context.Context, userID string, startDate, endDate time.Time) (*UserAnalyticsResponse, error)
}

type repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) Repository {
	return &repository{db: db}
}

// UpsertDailyMetric creates or updates a daily metric record
func (r *repository) UpsertDailyMetric(ctx context.Context, metric *DailyMetric) error {
	query := `
		INSERT INTO daily_metrics (
			metric_date, total_transactions, total_volume, total_fees,
			unique_users, successful_transactions, failed_transactions, avg_transaction_value
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (metric_date) DO UPDATE SET
			total_transactions = daily_metrics.total_transactions + EXCLUDED.total_transactions,
			total_volume = daily_metrics.total_volume + EXCLUDED.total_volume,
			total_fees = daily_metrics.total_fees + EXCLUDED.total_fees,
			unique_users = EXCLUDED.unique_users,
			successful_transactions = daily_metrics.successful_transactions + EXCLUDED.successful_transactions,
			failed_transactions = daily_metrics.failed_transactions + EXCLUDED.failed_transactions,
			avg_transaction_value = CASE 
				WHEN (daily_metrics.total_transactions + EXCLUDED.total_transactions) > 0 
				THEN (daily_metrics.total_volume + EXCLUDED.total_volume) / (daily_metrics.total_transactions + EXCLUDED.total_transactions)
				ELSE 0
			END,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		metric.MetricDate,
		metric.TotalTransactions,
		metric.TotalVolume,
		metric.TotalFees,
		metric.UniqueUsers,
		metric.SuccessfulTransactions,
		metric.FailedTransactions,
		metric.AvgTransactionValue,
	).Scan(&metric.ID, &metric.CreatedAt, &metric.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to upsert daily metric: %w", err)
	}

	return nil
}

// GetDailyMetrics retrieves daily metrics within a date range
func (r *repository) GetDailyMetrics(ctx context.Context, startDate, endDate time.Time) ([]*DailyMetric, error) {
	query := `
		SELECT id, metric_date, total_transactions, total_volume, total_fees,
			   unique_users, successful_transactions, failed_transactions, 
			   avg_transaction_value, created_at, updated_at
		FROM daily_metrics
		WHERE metric_date BETWEEN $1 AND $2
		ORDER BY metric_date DESC
	`

	rows, err := r.db.QueryContext(ctx, query, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily metrics: %w", err)
	}
	defer rows.Close()

	var metrics []*DailyMetric
	for rows.Next() {
		var m DailyMetric
		err := rows.Scan(
			&m.ID, &m.MetricDate, &m.TotalTransactions, &m.TotalVolume, &m.TotalFees,
			&m.UniqueUsers, &m.SuccessfulTransactions, &m.FailedTransactions,
			&m.AvgTransactionValue, &m.CreatedAt, &m.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan daily metric: %w", err)
		}
		metrics = append(metrics, &m)
	}

	return metrics, rows.Err()
}

// GetDailyMetricByDate retrieves a specific daily metric
func (r *repository) GetDailyMetricByDate(ctx context.Context, date time.Time) (*DailyMetric, error) {
	query := `
		SELECT id, metric_date, total_transactions, total_volume, total_fees,
			   unique_users, successful_transactions, failed_transactions, 
			   avg_transaction_value, created_at, updated_at
		FROM daily_metrics
		WHERE metric_date = $1
	`

	var m DailyMetric
	err := r.db.QueryRowContext(ctx, query, date).Scan(
		&m.ID, &m.MetricDate, &m.TotalTransactions, &m.TotalVolume, &m.TotalFees,
		&m.UniqueUsers, &m.SuccessfulTransactions, &m.FailedTransactions,
		&m.AvgTransactionValue, &m.CreatedAt, &m.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get daily metric: %w", err)
	}

	return &m, nil
}

// UpsertHourlyMetric creates or updates an hourly metric record
func (r *repository) UpsertHourlyMetric(ctx context.Context, metric *HourlyMetric) error {
	query := `
		INSERT INTO hourly_metrics (
			metric_hour, total_transactions, total_volume, total_fees,
			unique_users, successful_transactions, failed_transactions, 
			avg_transaction_value, max_transaction_value, min_transaction_value, avg_processing_time_ms
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (metric_hour) DO UPDATE SET
			total_transactions = hourly_metrics.total_transactions + EXCLUDED.total_transactions,
			total_volume = hourly_metrics.total_volume + EXCLUDED.total_volume,
			total_fees = hourly_metrics.total_fees + EXCLUDED.total_fees,
			unique_users = EXCLUDED.unique_users,
			successful_transactions = hourly_metrics.successful_transactions + EXCLUDED.successful_transactions,
			failed_transactions = hourly_metrics.failed_transactions + EXCLUDED.failed_transactions,
			avg_transaction_value = CASE 
				WHEN (hourly_metrics.total_transactions + EXCLUDED.total_transactions) > 0 
				THEN (hourly_metrics.total_volume + EXCLUDED.total_volume) / (hourly_metrics.total_transactions + EXCLUDED.total_transactions)
				ELSE 0
			END,
			max_transaction_value = GREATEST(hourly_metrics.max_transaction_value, EXCLUDED.max_transaction_value),
			min_transaction_value = LEAST(hourly_metrics.min_transaction_value, EXCLUDED.min_transaction_value),
			avg_processing_time_ms = ((hourly_metrics.avg_processing_time_ms * hourly_metrics.total_transactions) + 
									 (EXCLUDED.avg_processing_time_ms * EXCLUDED.total_transactions)) / 
									 (hourly_metrics.total_transactions + EXCLUDED.total_transactions),
			updated_at = CURRENT_TIMESTAMP
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		metric.MetricHour,
		metric.TotalTransactions,
		metric.TotalVolume,
		metric.TotalFees,
		metric.UniqueUsers,
		metric.SuccessfulTransactions,
		metric.FailedTransactions,
		metric.AvgTransactionValue,
		metric.MaxTransactionValue,
		metric.MinTransactionValue,
		metric.AvgProcessingTimeMs,
	).Scan(&metric.ID, &metric.CreatedAt, &metric.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to upsert hourly metric: %w", err)
	}

	return nil
}

// GetHourlyMetrics retrieves hourly metrics within a time range
func (r *repository) GetHourlyMetrics(ctx context.Context, startTime, endTime time.Time) ([]*HourlyMetric, error) {
	query := `
		SELECT id, metric_hour, total_transactions, total_volume, total_fees,
			   unique_users, successful_transactions, failed_transactions, 
			   avg_transaction_value, max_transaction_value, min_transaction_value,
			   avg_processing_time_ms, created_at, updated_at
		FROM hourly_metrics
		WHERE metric_hour BETWEEN $1 AND $2
		ORDER BY metric_hour DESC
	`

	rows, err := r.db.QueryContext(ctx, query, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get hourly metrics: %w", err)
	}
	defer rows.Close()

	var metrics []*HourlyMetric
	for rows.Next() {
		var m HourlyMetric
		err := rows.Scan(
			&m.ID, &m.MetricHour, &m.TotalTransactions, &m.TotalVolume, &m.TotalFees,
			&m.UniqueUsers, &m.SuccessfulTransactions, &m.FailedTransactions,
			&m.AvgTransactionValue, &m.MaxTransactionValue, &m.MinTransactionValue,
			&m.AvgProcessingTimeMs, &m.CreatedAt, &m.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan hourly metric: %w", err)
		}
		metrics = append(metrics, &m)
	}

	return metrics, rows.Err()
}

// GetHourlyMetricByHour retrieves a specific hourly metric
func (r *repository) GetHourlyMetricByHour(ctx context.Context, hour time.Time) (*HourlyMetric, error) {
	query := `
		SELECT id, metric_hour, total_transactions, total_volume, total_fees,
			   unique_users, successful_transactions, failed_transactions, 
			   avg_transaction_value, max_transaction_value, min_transaction_value,
			   avg_processing_time_ms, created_at, updated_at
		FROM hourly_metrics
		WHERE metric_hour = $1
	`

	var m HourlyMetric
	err := r.db.QueryRowContext(ctx, query, hour).Scan(
		&m.ID, &m.MetricHour, &m.TotalTransactions, &m.TotalVolume, &m.TotalFees,
		&m.UniqueUsers, &m.SuccessfulTransactions, &m.FailedTransactions,
		&m.AvgTransactionValue, &m.MaxTransactionValue, &m.MinTransactionValue,
		&m.AvgProcessingTimeMs, &m.CreatedAt, &m.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get hourly metric: %w", err)
	}

	return &m, nil
}

// UpsertUserSnapshot creates or updates a user snapshot
func (r *repository) UpsertUserSnapshot(ctx context.Context, snapshot *UserSnapshot) error {
	query := `
		INSERT INTO user_snapshots (
			user_id, snapshot_date, total_sent, total_received, transaction_count,
			sent_count, received_count, total_fees_paid, last_transaction_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (user_id, snapshot_date) DO UPDATE SET
			total_sent = user_snapshots.total_sent + EXCLUDED.total_sent,
			total_received = user_snapshots.total_received + EXCLUDED.total_received,
			transaction_count = user_snapshots.transaction_count + EXCLUDED.transaction_count,
			sent_count = user_snapshots.sent_count + EXCLUDED.sent_count,
			received_count = user_snapshots.received_count + EXCLUDED.received_count,
			total_fees_paid = user_snapshots.total_fees_paid + EXCLUDED.total_fees_paid,
			last_transaction_at = GREATEST(user_snapshots.last_transaction_at, EXCLUDED.last_transaction_at),
			updated_at = CURRENT_TIMESTAMP
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		snapshot.UserID,
		snapshot.SnapshotDate,
		snapshot.TotalSent,
		snapshot.TotalReceived,
		snapshot.TransactionCount,
		snapshot.SentCount,
		snapshot.ReceivedCount,
		snapshot.TotalFeesPaid,
		snapshot.LastTransactionAt,
	).Scan(&snapshot.ID, &snapshot.CreatedAt, &snapshot.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to upsert user snapshot: %w", err)
	}

	return nil
}

// GetUserSnapshots retrieves user snapshots within a date range
func (r *repository) GetUserSnapshots(ctx context.Context, userID string, startDate, endDate time.Time) ([]*UserSnapshot, error) {
	query := `
		SELECT id, user_id, snapshot_date, total_sent, total_received, transaction_count,
			   sent_count, received_count, total_fees_paid, last_transaction_at,
			   created_at, updated_at
		FROM user_snapshots
		WHERE user_id = $1 AND snapshot_date BETWEEN $2 AND $3
		ORDER BY snapshot_date DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get user snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []*UserSnapshot
	for rows.Next() {
		var s UserSnapshot
		err := rows.Scan(
			&s.ID, &s.UserID, &s.SnapshotDate, &s.TotalSent, &s.TotalReceived,
			&s.TransactionCount, &s.SentCount, &s.ReceivedCount, &s.TotalFeesPaid,
			&s.LastTransactionAt, &s.CreatedAt, &s.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user snapshot: %w", err)
		}
		snapshots = append(snapshots, &s)
	}

	return snapshots, rows.Err()
}

// GetUserSnapshotByDate retrieves a specific user snapshot
func (r *repository) GetUserSnapshotByDate(ctx context.Context, userID string, date time.Time) (*UserSnapshot, error) {
	query := `
		SELECT id, user_id, snapshot_date, total_sent, total_received, transaction_count,
			   sent_count, received_count, total_fees_paid, last_transaction_at,
			   created_at, updated_at
		FROM user_snapshots
		WHERE user_id = $1 AND snapshot_date = $2
	`

	var s UserSnapshot
	err := r.db.QueryRowContext(ctx, query, userID, date).Scan(
		&s.ID, &s.UserID, &s.SnapshotDate, &s.TotalSent, &s.TotalReceived,
		&s.TransactionCount, &s.SentCount, &s.ReceivedCount, &s.TotalFeesPaid,
		&s.LastTransactionAt, &s.CreatedAt, &s.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user snapshot: %w", err)
	}

	return &s, nil
}

func (r *repository) CreateEventLog(ctx context.Context, log *EventProcessingLog) error {
	query := `
		INSERT INTO event_processing_log (
			event_id, event_type, topic, partition, "offset", event_data,
			processed_at, processing_time_ms, status, error_message, retry_count
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at
	`

	err := r.db.QueryRowContext(ctx, query,
		log.EventID,
		log.EventType,
		log.Topic,
		log.Partition,
		log.Offset,
		log.EventData,
		log.ProcessedAt,
		log.ProcessingTimeMs,
		log.Status,
		log.ErrorMessage,
		log.RetryCount,
	).Scan(&log.ID, &log.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create event log: %w", err)
	}

	return nil
}

func (r *repository) GetEventLogByEventID(ctx context.Context, eventID string) (*EventProcessingLog, error) {
	query := `
		SELECT id, event_id, event_type, topic, partition, "offset", event_data,
			   processed_at, processing_time_ms, status, error_message, retry_count, created_at
		FROM event_processing_log
		WHERE event_id = $1
	`

	var log EventProcessingLog
	err := r.db.QueryRowContext(ctx, query, eventID).Scan(
		&log.ID, &log.EventID, &log.EventType, &log.Topic, &log.Partition, &log.Offset,
		&log.EventData, &log.ProcessedAt, &log.ProcessingTimeMs, &log.Status,
		&log.ErrorMessage, &log.RetryCount, &log.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get event log: %w", err)
	}

	return &log, nil
}

// UpdateEventLogStatus updates the status of an event log
func (r *repository) UpdateEventLogStatus(ctx context.Context, eventID, status string, errorMsg *string, retryCount int) error {
	query := `
		UPDATE event_processing_log
		SET status = $1, error_message = $2, retry_count = $3
		WHERE event_id = $4
	`

	_, err := r.db.ExecContext(ctx, query, status, errorMsg, retryCount, eventID)
	if err != nil {
		return fmt.Errorf("failed to update event log status: %w", err)
	}

	return nil
}

// GetMetricsSummary returns aggregated metrics summary
func (r *repository) GetMetricsSummary(ctx context.Context, startDate, endDate time.Time, period string) (*MetricsSummaryResponse, error) {
	var query string

	if period == "hourly" {
		query = `
			SELECT 
				COALESCE(SUM(total_transactions), 0) as total_transactions,
				COALESCE(SUM(total_volume), 0) as total_volume,
				COALESCE(SUM(total_fees), 0) as total_fees,
				COALESCE(MAX(unique_users), 0) as unique_users,
				CASE 
					WHEN SUM(total_transactions) > 0 
					THEN (SUM(successful_transactions)::float / SUM(total_transactions)::float) * 100
					ELSE 0
				END as success_rate,
				CASE 
					WHEN SUM(total_transactions) > 0 
					THEN SUM(total_volume) / SUM(total_transactions)
					ELSE 0
				END as avg_transaction_size
			FROM hourly_metrics
			WHERE metric_hour BETWEEN $1 AND $2
		`
	} else {
		query = `
			SELECT 
				COALESCE(SUM(total_transactions), 0) as total_transactions,
				COALESCE(SUM(total_volume), 0) as total_volume,
				COALESCE(SUM(total_fees), 0) as total_fees,
				COALESCE(MAX(unique_users), 0) as unique_users,
				CASE 
					WHEN SUM(total_transactions) > 0 
					THEN (SUM(successful_transactions)::float / SUM(total_transactions)::float) * 100
					ELSE 0
				END as success_rate,
				CASE 
					WHEN SUM(total_transactions) > 0 
					THEN SUM(total_volume) / SUM(total_transactions)
					ELSE 0
				END as avg_transaction_size
			FROM daily_metrics
			WHERE metric_date BETWEEN $1 AND $2
		`
	}

	var summary MetricsSummaryResponse
	err := r.db.QueryRowContext(ctx, query, startDate, endDate).Scan(
		&summary.TotalTransactions,
		&summary.TotalVolume,
		&summary.TotalFees,
		&summary.UniqueUsers,
		&summary.SuccessRate,
		&summary.AvgTransactionSize,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get metrics summary: %w", err)
	}

	summary.Period = period
	return &summary, nil
}

// GetUserAnalytics returns user-specific analytics
func (r *repository) GetUserAnalytics(ctx context.Context, userID string, startDate, endDate time.Time) (*UserAnalyticsResponse, error) {
	query := `
		SELECT 
			user_id,
			COALESCE(SUM(total_sent), 0) as total_sent,
			COALESCE(SUM(total_received), 0) as total_received,
			COALESCE(SUM(transaction_count), 0) as transaction_count,
			COALESCE(SUM(total_fees_paid), 0) as total_fees_paid,
			MAX(last_transaction_at) as last_transaction_at
		FROM user_snapshots
		WHERE user_id = $1 AND snapshot_date BETWEEN $2 AND $3
		GROUP BY user_id
	`

	var analytics UserAnalyticsResponse
	var lastTxAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, userID, startDate, endDate).Scan(
		&analytics.UserID,
		&analytics.TotalSent,
		&analytics.TotalReceived,
		&analytics.TransactionCount,
		&analytics.TotalFeesPaid,
		&lastTxAt,
	)

	if err == sql.ErrNoRows {
		// Return empty analytics for user with no data
		return &UserAnalyticsResponse{
			UserID:           userID,
			Period:           fmt.Sprintf("%s to %s", startDate.Format("2006-01-02"), endDate.Format("2006-01-02")),
			TotalSent:        0,
			TotalReceived:    0,
			NetAmount:        0,
			TransactionCount: 0,
			TotalFeesPaid:    0,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user analytics: %w", err)
	}

	if lastTxAt.Valid {
		analytics.LastTransactionAt = &lastTxAt.Time
	}

	analytics.NetAmount = analytics.TotalReceived - analytics.TotalSent
	analytics.Period = fmt.Sprintf("%s to %s", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	return &analytics, nil
}

// Helper function to marshal event data to JSON
func MarshalEventData(event interface{}) ([]byte, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event data: %w", err)
	}
	return data, nil
}