-- Outbox events table for reliable event publishing
-- NOTE: This table ensures exactly-once delivery to Kafka
-- Events are saved here in the same transaction as business data
-- A background worker publishes them to Kafka asynchronously

CREATE TABLE IF NOT EXISTS outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_id VARCHAR(255) NOT NULL,           -- e.g., wallet_id, transaction_id
    event_type VARCHAR(100) NOT NULL,              -- e.g., "wallet.balance_updated"
    topic VARCHAR(100) NOT NULL,                   -- Kafka topic name
    payload JSONB NOT NULL,                        -- Event data as JSON
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, published, failed
    attempts INT NOT NULL DEFAULT 0,               -- Retry counter
    last_error TEXT,                               -- Error message if failed
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    published_at TIMESTAMP WITH TIME ZONE          -- When successfully published to Kafka
);

-- Index for efficient polling of pending events
CREATE INDEX IF NOT EXISTS idx_outbox_status ON outbox_events(status, created_at);

-- Index for querying events by aggregate (for debugging)
CREATE INDEX IF NOT EXISTS idx_outbox_aggregate ON outbox_events(aggregate_id);

-- Index for cleanup of old published events
CREATE INDEX IF NOT EXISTS idx_outbox_published_at ON outbox_events(published_at);

-- NOTE: You should periodically clean up old published events to prevent table bloat
-- Example cleanup query (run daily):
-- DELETE FROM outbox_events WHERE status = 'published' AND published_at < NOW() - INTERVAL '30 days';