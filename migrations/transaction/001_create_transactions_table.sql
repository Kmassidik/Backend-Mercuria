-- Transactions table (individual transfers)
-- NOTE: Stores all types of transfers (P2P, batch items, scheduled)
CREATE TABLE IF NOT EXISTS transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_wallet_id VARCHAR(255) NOT NULL,           -- Source wallet
    to_wallet_id VARCHAR(255) NOT NULL,             -- Destination wallet
    amount NUMERIC(20, 4) NOT NULL,                 -- Transfer amount
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',     -- Currency code
    type VARCHAR(20) NOT NULL,                      -- p2p, batch, scheduled
    status VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending, completed, failed, scheduled, cancelled
    description TEXT,                                -- Optional memo
    idempotency_key VARCHAR(255) UNIQUE NOT NULL,   -- Prevents duplicates
    scheduled_at TIMESTAMP WITH TIME ZONE,           -- For scheduled transfers
    processed_at TIMESTAMP WITH TIME ZONE,           -- When transfer completed
    failure_reason TEXT,                             -- Error message if failed
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT positive_amount CHECK (amount > 0),
    CONSTRAINT different_wallets CHECK (from_wallet_id != to_wallet_id)
);

-- Indexes for transaction queries
CREATE INDEX IF NOT EXISTS idx_transactions_from_wallet ON transactions(from_wallet_id);
CREATE INDEX IF NOT EXISTS idx_transactions_to_wallet ON transactions(to_wallet_id);
CREATE INDEX IF NOT EXISTS idx_transactions_status ON transactions(status);
CREATE INDEX IF NOT EXISTS idx_transactions_idempotency ON transactions(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_transactions_created ON transactions(created_at DESC);

-- Index for scheduled transaction worker
-- NOTE: Only indexes scheduled transactions for efficient polling
CREATE INDEX IF NOT EXISTS idx_transactions_scheduled 
    ON transactions(scheduled_at) 
    WHERE status = 'scheduled';

-- Batch transactions table
-- NOTE: Groups multiple transfers into a single batch operation
CREATE TABLE IF NOT EXISTS batch_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_wallet_id VARCHAR(255) NOT NULL,           -- Single source wallet
    total_amount NUMERIC(20, 4) NOT NULL,           -- Sum of all transfers
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending, completed, failed
    idempotency_key VARCHAR(255) UNIQUE NOT NULL,   -- Prevents duplicate batches
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT positive_total CHECK (total_amount > 0)
);

-- Indexes for batch queries
CREATE INDEX IF NOT EXISTS idx_batch_from_wallet ON batch_transactions(from_wallet_id);
CREATE INDEX IF NOT EXISTS idx_batch_status ON batch_transactions(status);
CREATE INDEX IF NOT EXISTS idx_batch_idempotency ON batch_transactions(idempotency_key);

-- Trigger to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_transactions_updated_at 
    BEFORE UPDATE ON transactions
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_batch_transactions_updated_at 
    BEFORE UPDATE ON batch_transactions
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

-- NOTE: Individual batch transfers are stored in the transactions table
-- with type='batch' and linked via metadata or description field
-- This simplifies querying and maintains consistency