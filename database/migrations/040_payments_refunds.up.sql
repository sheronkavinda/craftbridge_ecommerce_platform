-- 040_payments_refunds.up.sql
-- Member 05 (Payments, Refunds & Receipts) schema migration.
-- Uses plain TEXT columns for cross-module IDs (order_id, user_id, seller_id) so this
-- migration does not depend on foreign tables owned by other modules.

-- 1. Payment Attempts
CREATE TABLE IF NOT EXISTS payment_attempts (
    id TEXT PRIMARY KEY,
    order_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    provider TEXT NOT NULL DEFAULT 'payhere_sandbox',
    provider_reference TEXT,
    amount_cents BIGINT NOT NULL CHECK (amount_cents >= 0),
    currency TEXT NOT NULL DEFAULT 'USD',
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'SUCCESS', 'FAILED', 'EXPIRED', 'CANCELLED')),
    failure_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_payment_attempts_order_id
    ON payment_attempts (order_id);

CREATE INDEX IF NOT EXISTS idx_payment_attempts_user_id
    ON payment_attempts (user_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_payment_attempts_provider_ref
    ON payment_attempts (provider, provider_reference)
    WHERE provider_reference IS NOT NULL;

-- 2. Payment Events (Webhook / Callback Idempotency & Audit Log)
CREATE TABLE IF NOT EXISTS payment_events (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    provider_event_id TEXT NOT NULL,
    attempt_id TEXT REFERENCES payment_attempts(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    signature_valid BOOLEAN NOT NULL DEFAULT FALSE,
    processed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_payment_events_provider_event UNIQUE (provider, provider_event_id)
);

CREATE INDEX IF NOT EXISTS idx_payment_events_attempt_id
    ON payment_events (attempt_id);

-- 3. Refunds
-- Compatible with both Member 05 (amount_cents, decided_at, decided_by, payment_attempt_id, etc.)
-- and Member 06 admin/report queries (amount, reviewed_at, reason, status).
CREATE TABLE IF NOT EXISTS refunds (
    id TEXT PRIMARY KEY,
    order_id TEXT NOT NULL,
    payment_attempt_id TEXT REFERENCES payment_attempts(id) ON DELETE SET NULL,
    requested_by TEXT NOT NULL DEFAULT '',
    seller_id TEXT,
    reason TEXT NOT NULL DEFAULT '',
    details TEXT NOT NULL DEFAULT '',
    amount NUMERIC(12, 2) NOT NULL DEFAULT 0,
    amount_cents BIGINT NOT NULL DEFAULT 0 CHECK (amount_cents >= 0),
    currency TEXT NOT NULL DEFAULT 'USD',
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'APPROVED', 'PROCESSED', 'COMPLETED', 'REJECTED')),
    provider_refund_reference TEXT,
    idempotency_key TEXT UNIQUE,
    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_at TIMESTAMPTZ,
    decided_at TIMESTAMPTZ,
    decided_by TEXT,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Ensure columns exist if the refunds table was created earlier by a dev seed script
ALTER TABLE refunds ADD COLUMN IF NOT EXISTS payment_attempt_id TEXT REFERENCES payment_attempts(id) ON DELETE SET NULL;
ALTER TABLE refunds ADD COLUMN IF NOT EXISTS requested_by TEXT NOT NULL DEFAULT '';
ALTER TABLE refunds ADD COLUMN IF NOT EXISTS seller_id TEXT;
ALTER TABLE refunds ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT '';
ALTER TABLE refunds ADD COLUMN IF NOT EXISTS details TEXT NOT NULL DEFAULT '';
ALTER TABLE refunds ADD COLUMN IF NOT EXISTS amount NUMERIC(12, 2) NOT NULL DEFAULT 0;
ALTER TABLE refunds ADD COLUMN IF NOT EXISTS amount_cents BIGINT NOT NULL DEFAULT 0;
ALTER TABLE refunds ADD COLUMN IF NOT EXISTS currency TEXT NOT NULL DEFAULT 'USD';
ALTER TABLE refunds ADD COLUMN IF NOT EXISTS provider_refund_reference TEXT;
ALTER TABLE refunds ADD COLUMN IF NOT EXISTS idempotency_key TEXT UNIQUE;
ALTER TABLE refunds ADD COLUMN IF NOT EXISTS requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE refunds ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ;
ALTER TABLE refunds ADD COLUMN IF NOT EXISTS decided_at TIMESTAMPTZ;
ALTER TABLE refunds ADD COLUMN IF NOT EXISTS decided_by TEXT;
ALTER TABLE refunds ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;
ALTER TABLE refunds ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE refunds ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE INDEX IF NOT EXISTS idx_refunds_order_id
    ON refunds (order_id);

CREATE INDEX IF NOT EXISTS idx_refunds_status
    ON refunds (status);

CREATE INDEX IF NOT EXISTS idx_refunds_payment_attempt_id
    ON refunds (payment_attempt_id);

-- 4. Receipts
CREATE TABLE IF NOT EXISTS receipts (
    id TEXT PRIMARY KEY,
    order_id TEXT NOT NULL UNIQUE,
    payment_attempt_id TEXT NOT NULL REFERENCES payment_attempts(id) ON DELETE RESTRICT,
    user_id TEXT NOT NULL,
    receipt_number TEXT NOT NULL UNIQUE,
    amount_cents BIGINT NOT NULL CHECK (amount_cents >= 0),
    currency TEXT NOT NULL DEFAULT 'USD',
    issued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_receipts_user_id
    ON receipts (user_id);

CREATE INDEX IF NOT EXISTS idx_receipts_payment_attempt_id
    ON receipts (payment_attempt_id);

