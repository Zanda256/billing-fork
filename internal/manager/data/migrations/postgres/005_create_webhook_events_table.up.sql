-- 005_create_webhook_events_table.up.sql
-- Adds the billing.webhook_events table used for processor-agnostic webhook persistence

CREATE TABLE IF NOT EXISTS billing.webhook_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    processor TEXT NOT NULL,
    event_id TEXT,
    event_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    raw_payload TEXT NOT NULL,
    headers JSONB,
    ip_address TEXT NOT NULL,
    signature TEXT,
    signature_valid BOOLEAN,
    processing_result JSONB,
    error_message TEXT,
    subscription_id UUID,
    user_id TEXT,
    processing_attempts INTEGER NOT NULL DEFAULT 0,
    last_attempt_at TIMESTAMPTZ,
    next_attempt_at TIMESTAMPTZ,
    received_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    processed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE INDEX IF NOT EXISTS idx_webhook_events_status ON billing.webhook_events(status);
CREATE INDEX IF NOT EXISTS idx_webhook_events_processor ON billing.webhook_events(processor, event_type);
CREATE INDEX IF NOT EXISTS idx_webhook_events_next_attempt ON billing.webhook_events(next_attempt_at) WHERE status IN ('pending', 'failed');
CREATE INDEX IF NOT EXISTS idx_webhook_events_event_id ON billing.webhook_events(event_id) WHERE event_id IS NOT NULL;
