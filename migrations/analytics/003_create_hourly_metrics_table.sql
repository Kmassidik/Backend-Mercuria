-- +goose Up
-- Create hourly_metrics table for real-time monitoring and alerting
CREATE TABLE IF NOT EXISTS hourly_metrics (
    id BIGSERIAL PRIMARY KEY,
    metric_hour TIMESTAMP NOT NULL,
    total_transactions BIGINT DEFAULT 0 NOT NULL,
    total_volume NUMERIC(20, 4) DEFAULT 0 NOT NULL,
    total_fees NUMERIC(20, 4) DEFAULT 0 NOT NULL,
    unique_users BIGINT DEFAULT 0 NOT NULL,
    successful_transactions BIGINT DEFAULT 0 NOT NULL,
    failed_transactions BIGINT DEFAULT 0 NOT NULL,
    avg_transaction_value NUMERIC(20, 4) DEFAULT 0 NOT NULL,
    max_transaction_value NUMERIC(20, 4) DEFAULT 0 NOT NULL,
    min_transaction_value NUMERIC(20, 4) DEFAULT 0 NOT NULL,
    avg_processing_time_ms INTEGER DEFAULT 0 NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT unique_metric_hour UNIQUE (metric_hour)
);

-- Index for time-based queries
CREATE INDEX idx_hourly_metrics_hour ON hourly_metrics(metric_hour DESC);

-- Index for volume-based queries
CREATE INDEX idx_hourly_metrics_volume ON hourly_metrics(total_volume DESC);

-- Index for recent metrics
CREATE INDEX idx_hourly_metrics_created_at ON hourly_metrics(created_at DESC);

-- Create trigger for updated_at
CREATE OR REPLACE FUNCTION update_hourly_metrics_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_hourly_metrics_updated_at
    BEFORE UPDATE ON hourly_metrics
    FOR EACH ROW
    EXECUTE FUNCTION update_hourly_metrics_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS trigger_hourly_metrics_updated_at ON hourly_metrics;
DROP FUNCTION IF EXISTS update_hourly_metrics_updated_at();
DROP INDEX IF EXISTS idx_hourly_metrics_created_at;
DROP INDEX IF EXISTS idx_hourly_metrics_volume;
DROP INDEX IF EXISTS idx_hourly_metrics_hour;
DROP TABLE IF EXISTS hourly_metrics;