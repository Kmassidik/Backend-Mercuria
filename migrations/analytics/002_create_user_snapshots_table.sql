-- +goose Down
DROP TRIGGER IF EXISTS trigger_user_snapshots_updated_at ON user_snapshots;
DROP FUNCTION IF EXISTS update_user_snapshots_updated_at();
DROP INDEX IF EXISTS idx_user_snapshots_last_transaction;
DROP INDEX IF EXISTS idx_user_snapshots_user_date;
DROP INDEX IF EXISTS idx_user_snapshots_date;
DROP INDEX IF EXISTS idx_user_snapshots_user_id;
DROP TABLE IF EXISTS user_snapshots;

-- +goose Up
-- Create user_snapshots table for per-user aggregated statistics
CREATE TABLE IF NOT EXISTS user_snapshots (
    id BIGSERIAL PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL,
    snapshot_date DATE NOT NULL,
    total_sent NUMERIC(20, 4) DEFAULT 0 NOT NULL,
    total_received NUMERIC(20, 4) DEFAULT 0 NOT NULL,
    transaction_count BIGINT DEFAULT 0 NOT NULL,
    sent_count BIGINT DEFAULT 0 NOT NULL,
    received_count BIGINT DEFAULT 0 NOT NULL,
    total_fees_paid NUMERIC(20, 4) DEFAULT 0 NOT NULL,
    last_transaction_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT unique_user_snapshot_date UNIQUE (user_id, snapshot_date)
);

-- Index for user-based queries
CREATE INDEX idx_user_snapshots_user_id ON user_snapshots(user_id);

-- Index for date-based queries
CREATE INDEX idx_user_snapshots_date ON user_snapshots(snapshot_date DESC);

-- Composite index for user + date queries
CREATE INDEX idx_user_snapshots_user_date ON user_snapshots(user_id, snapshot_date DESC);

-- Index for transaction timestamp queries
CREATE INDEX idx_user_snapshots_last_transaction ON user_snapshots(last_transaction_at DESC) WHERE last_transaction_at IS NOT NULL;

-- Create trigger for updated_at
CREATE OR REPLACE FUNCTION update_user_snapshots_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_user_snapshots_updated_at
    BEFORE UPDATE ON user_snapshots
    FOR EACH ROW
    EXECUTE FUNCTION update_user_snapshots_updated_at();

