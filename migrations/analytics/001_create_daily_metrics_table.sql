-- +goose Down
DROP TRIGGER IF EXISTS trigger_daily_metrics_updated_at ON daily_metrics;
DROP FUNCTION IF EXISTS update_daily_metrics_updated_at();
DROP INDEX IF EXISTS idx_daily_metrics_created_at;
DROP INDEX IF EXISTS idx_daily_metrics_date;
DROP TABLE IF EXISTS daily_metrics;

-- +goose Up
-- Create daily_metrics table for aggregated daily statistics
CREATE TABLE IF NOT EXISTS daily_metrics (
    id BIGSERIAL PRIMARY KEY,
    metric_date DATE NOT NULL,
    total_transactions BIGINT DEFAULT 0 NOT NULL,
    total_volume NUMERIC(20, 4) DEFAULT 0 NOT NULL,
    total_fees NUMERIC(20, 4) DEFAULT 0 NOT NULL,
    unique_users BIGINT DEFAULT 0 NOT NULL,
    successful_transactions BIGINT DEFAULT 0 NOT NULL,
    failed_transactions BIGINT DEFAULT 0 NOT NULL,
    avg_transaction_value NUMERIC(20, 4) DEFAULT 0 NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT unique_metric_date UNIQUE (metric_date)
);

-- Index for fast date-based queries
CREATE INDEX idx_daily_metrics_date ON daily_metrics(metric_date DESC);

-- Index for time-based queries
CREATE INDEX idx_daily_metrics_created_at ON daily_metrics(created_at DESC);

-- Create trigger for updated_at
CREATE OR REPLACE FUNCTION update_daily_metrics_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_daily_metrics_updated_at
    BEFORE UPDATE ON daily_metrics
    FOR EACH ROW
    EXECUTE FUNCTION update_daily_metrics_updated_at();

