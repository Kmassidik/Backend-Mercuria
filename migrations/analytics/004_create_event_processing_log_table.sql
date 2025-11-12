-- +goose Up
-- Create event_processing_log table for tracking processed Kafka events (idempotency)
CREATE TABLE IF NOT EXISTS event_processing_log (
    id BIGSERIAL PRIMARY KEY,
    event_id VARCHAR(36) NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    topic VARCHAR(100) NOT NULL,
    partition INTEGER NOT NULL,
    offset BIGINT NOT NULL,
    event_data JSONB,
    processed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processing_time_ms INTEGER DEFAULT 0 NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'processed',
    error_message TEXT,
    retry_count INTEGER DEFAULT 0 NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT unique_event_id UNIQUE (event_id)
);

-- Index for event_id lookups (idempotency check)
CREATE INDEX idx_event_processing_event_id ON event_processing_log(event_id);

-- Index for event_type queries
CREATE INDEX idx_event_processing_event_type ON event_processing_log(event_type);

-- Composite index for topic + partition + offset
CREATE INDEX idx_event_processing_topic_offset ON event_processing_log(topic, partition, offset DESC);

-- Index for status-based queries
CREATE INDEX idx_event_processing_status ON event_processing_log(status) WHERE status != 'processed';

-- Index for time-based queries
CREATE INDEX idx_event_processing_processed_at ON event_processing_log(processed_at DESC);

-- Index for error tracking
CREATE INDEX idx_event_processing_errors ON event_processing_log(processed_at DESC) WHERE error_message IS NOT NULL;

-- Partial index for recent failed events
CREATE INDEX idx_event_processing_recent_failures ON event_processing_log(processed_at DESC, retry_count) 
WHERE status = 'failed' AND processed_at > CURRENT_TIMESTAMP - INTERVAL '24 hours';

-- +goose Down
DROP INDEX IF EXISTS idx_event_processing_recent_failures;
DROP INDEX IF EXISTS idx_event_processing_errors;
DROP INDEX IF EXISTS idx_event_processing_processed_at;
DROP INDEX IF EXISTS idx_event_processing_status;
DROP INDEX IF EXISTS idx_event_processing_topic_offset;
DROP INDEX IF EXISTS idx_event_processing_event_type;
DROP INDEX IF EXISTS idx_event_processing_event_id;
DROP TABLE IF EXISTS event_processing_log;