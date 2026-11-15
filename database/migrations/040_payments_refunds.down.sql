-- 040_payments_refunds.down.sql
-- Drops Member 05 tables in reverse dependency order.

DROP INDEX IF EXISTS idx_receipts_payment_attempt_id;
DROP INDEX IF EXISTS idx_receipts_user_id;
DROP TABLE IF EXISTS receipts;

DROP INDEX IF EXISTS idx_refunds_payment_attempt_id;
DROP INDEX IF EXISTS idx_refunds_status;
DROP INDEX IF EXISTS idx_refunds_order_id;
DROP TABLE IF EXISTS refunds;

DROP INDEX IF EXISTS idx_payment_events_attempt_id;
DROP TABLE IF EXISTS payment_events;

DROP INDEX IF EXISTS uq_payment_attempts_provider_ref;
DROP INDEX IF EXISTS idx_payment_attempts_user_id;
DROP INDEX IF EXISTS idx_payment_attempts_order_id;
DROP TABLE IF EXISTS payment_attempts;

