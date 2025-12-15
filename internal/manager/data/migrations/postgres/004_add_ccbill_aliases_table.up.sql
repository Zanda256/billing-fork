SET lock_timeout = '10s';
SET statement_timeout = '300s';

CREATE TABLE IF NOT EXISTS billing.ccbill_username_aliases (
    alias TEXT PRIMARY KEY,
    user_id TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

COMMENT ON TABLE billing.ccbill_username_aliases IS 'Maps internal user IDs to CCBill-friendly username aliases (4-16 ASCII characters).';
COMMENT ON COLUMN billing.ccbill_username_aliases.alias IS 'CCBill-compliant username alias sent in FlexForm submissions.';
COMMENT ON COLUMN billing.ccbill_username_aliases.user_id IS 'Immutable Doujins user identifier (OIDC sub).';
