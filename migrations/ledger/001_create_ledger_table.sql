-- Ledger entries table (immutable audit trail)
-- NOTE: This is the immutable record of all financial movements
-- NEVER update or delete entries - append-only forever!

CREATE TABLE IF NOT EXISTS ledger_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id VARCHAR(255) NOT NULL,          -- Links to transaction service
    wallet_id VARCHAR(255) NOT NULL,               -- Which wallet this affects
    entry_type VARCHAR(10) NOT NULL,                -- 'debit' or 'credit'
    amount NUMERIC(20, 4) NOT NULL,                 -- Transaction amount
    currency VARCHAR(3) NOT NULL,                   -- USD, EUR, etc.
    balance NUMERIC(20, 4) NOT NULL,                -- Running balance after this entry
    description TEXT,                                -- Human-readable description
    metadata JSONB,                                  -- Additional context
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    -- Constraints for data integrity
    CHECK (entry_type IN ('debit', 'credit')),
    CHECK (amount > 0)
);

-- Indexes for efficient querying

-- Query ledger by transaction (shows complete double-entry)
CREATE INDEX IF NOT EXISTS idx_ledger_transaction 
    ON ledger_entries(transaction_id);

-- Query ledger by wallet (history with newest first)
CREATE INDEX IF NOT EXISTS idx_ledger_wallet 
    ON ledger_entries(wallet_id, created_at DESC);

-- Query ledger by date (audit trail)
CREATE INDEX IF NOT EXISTS idx_ledger_created 
    ON ledger_entries(created_at DESC);

-- Query by entry type (for analytics)
CREATE INDEX IF NOT EXISTS idx_ledger_entry_type 
    ON ledger_entries(entry_type);

-- Composite index for wallet stats
CREATE INDEX IF NOT EXISTS idx_ledger_wallet_type 
    ON ledger_entries(wallet_id, entry_type, created_at DESC);

-- CRITICAL: No triggers, no foreign keys that allow cascading deletes
-- Ledger must be immutable and independent for compliance

-- Optional: Partition by date for very large tables (millions of entries)
-- CREATE TABLE ledger_entries_2025_11 PARTITION OF ledger_entries
--     FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');

-- Compliance note: Retain ledger entries for minimum 7 years
-- Set up archival process, but NEVER delete entries