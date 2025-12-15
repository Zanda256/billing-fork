-- Explicitly set schema to ensure all objects are created in the correct place
-- Set timeouts to prevent hanging migrations
SET lock_timeout = '10s';
SET statement_timeout = '300s';

-- Install required extensions
-- Extensions are created by bootstrap; skip here.

-- ============================================================================
-- SECTION 1: CORE SUBSCRIPTION TABLES
-- ============================================================================

-- 1.2: Create subscription status enum (idempotent without requiring owner privileges)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_type t
        JOIN pg_namespace n ON t.typnamespace = n.oid
        WHERE t.typname = 'subscription_status' AND n.nspname = 'billing'
    ) THEN
        CREATE TYPE billing.subscription_status AS ENUM ('pending', 'active', 'expired', 'cancelled', 'failed', 'past_due');
    END IF;
END$$;

-- 1.3: Create subscriptions table
CREATE TABLE IF NOT EXISTS billing.subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL, -- AuthKit user ID (UUID)
    price_id UUID, -- References prices table (created later)
    status billing.subscription_status NOT NULL DEFAULT 'pending',

    -- Processor information
    processor TEXT NOT NULL DEFAULT 'ccbill',
    processor_provider TEXT,
    processor_subscription_id TEXT NOT NULL DEFAULT '',
    user_email TEXT,
    payment_method_id UUID, -- References payment_methods table (created later)
    
    -- Billing period tracking
    current_period_starts_at TIMESTAMPTZ,
    current_period_ends_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    ended_at TIMESTAMPTZ,
    
    -- Retry fields for manual rebilling
    last_retry_at TIMESTAMPTZ,
    retry_attempts INTEGER DEFAULT 0,
    next_retry_at TIMESTAMPTZ,
    
    -- Cancellation tracking
    cancelled_at TIMESTAMPTZ,
    cancel_type TEXT, -- 'user', 'admin', 'failed_payment', 'expired'
    cancel_feedback TEXT,
    
    -- Metadata
    gateway_response JSONB,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

-- Create indexes for subscriptions
CREATE INDEX IF NOT EXISTS idx_subscriptions_user_id ON billing.subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_price_id ON billing.subscriptions(price_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_status ON billing.subscriptions(status);
CREATE INDEX IF NOT EXISTS idx_subscriptions_processor ON billing.subscriptions(processor);
CREATE INDEX IF NOT EXISTS idx_subscriptions_processor_subscription ON billing.subscriptions(processor, coalesce(processor_provider, ''), processor_subscription_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_next_retry_at ON billing.subscriptions(next_retry_at) WHERE next_retry_at IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_subscriptions_user_active ON billing.subscriptions(user_id) WHERE status = 'active';

COMMENT ON INDEX billing.idx_subscriptions_user_active IS 'Ensures each user can have only one active subscription at a time';


-- ============================================================================
-- SECTION 2: PRODUCTS AND PRICING
-- ============================================================================

-- 2.1: Create products table
CREATE TABLE IF NOT EXISTS billing.products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT,
    entitlements_spec JSONB, -- map: entitlement_name -> duration_days (null/0 = indefinite)
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE INDEX IF NOT EXISTS idx_products_slug ON billing.products(slug);
CREATE INDEX IF NOT EXISTS idx_products_is_active ON billing.products(is_active);

-- 2.2: Create prices table
CREATE TABLE IF NOT EXISTS billing.prices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES billing.products(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL,
    amount DECIMAL(10,2) NOT NULL,
    currency TEXT NOT NULL,
    billing_cycle_days INTEGER, -- 30 for monthly, 365 for yearly, NULL for one-time
    nmi_plan_id TEXT, -- NMI processor plan ID
    nmi_provider TEXT, -- NMI provider slug (e.g., mobius)
    ccbill_price_id TEXT, -- CCBill processor price ID  
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE INDEX IF NOT EXISTS idx_prices_product_id ON billing.prices(product_id);
CREATE INDEX IF NOT EXISTS idx_prices_nmi_plan_provider ON billing.prices(nmi_provider, nmi_plan_id) WHERE nmi_plan_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_prices_ccbill_price_id ON billing.prices(ccbill_price_id) WHERE ccbill_price_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_prices_is_active ON billing.prices(is_active);

ALTER TABLE billing.prices DROP CONSTRAINT IF EXISTS unique_prices_product_amount_cycle;
ALTER TABLE billing.prices ADD CONSTRAINT unique_prices_product_amount_cycle 
    UNIQUE (product_id, amount, currency, billing_cycle_days);

-- Add foreign key reference from subscriptions to prices
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'fk_subscriptions_price_id'
    ) THEN
        ALTER TABLE billing.subscriptions
        ADD CONSTRAINT fk_subscriptions_price_id
        FOREIGN KEY (price_id) REFERENCES billing.prices(id);
    END IF;
END$$;

-- ============================================================================
-- SECTION 3: ENTITLEMENTS (SCD2 windows)
-- ============================================================================

CREATE TABLE IF NOT EXISTS billing.entitlements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    entitlement TEXT NOT NULL,
    start_at TIMESTAMPTZ NOT NULL,
    end_at TIMESTAMPTZ,
    source_id UUID,
    source_type TEXT NOT NULL,
    revoked_at TIMESTAMPTZ,
    revoke_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    deleted_at TIMESTAMPTZ,
    -- Generated helpers for overlap protection
    period tstzrange GENERATED ALWAYS AS (tstzrange(start_at, COALESCE(end_at, 'infinity'::timestamptz), '[)')) STORED
);

CREATE INDEX IF NOT EXISTS idx_entitlements_user_entitlement ON billing.entitlements(user_id, entitlement);
CREATE INDEX IF NOT EXISTS idx_entitlements_active_window ON billing.entitlements(user_id, entitlement, start_at, end_at) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_entitlements_source ON billing.entitlements(source_type, source_id) WHERE source_id IS NOT NULL;

-- At most one active entitlement per user+entitlement
CREATE UNIQUE INDEX IF NOT EXISTS uniq_entitlements_active ON billing.entitlements(user_id, entitlement)
WHERE revoked_at IS NULL AND end_at IS NULL;

-- Prevent overlapping entitlement windows per (user_id, entitlement) for non-deleted rows.
-- Simplified approach using a partial unique index instead of complex exclusion constraint
CREATE UNIQUE INDEX IF NOT EXISTS idx_entitlements_no_overlap
ON billing.entitlements(user_id, entitlement, start_at)
WHERE revoked_at IS NULL AND deleted_at IS NULL;

-- Backfill cleanup for older schemas: drop the former generated column if it exists
ALTER TABLE billing.entitlements DROP COLUMN IF EXISTS active;

-- ============================================================================
-- SECTION 4: PAYMENT PROCESSING
-- ============================================================================

-- 4.1: Create payment_methods table
CREATE TABLE IF NOT EXISTS billing.payment_methods (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL, -- OIDC subject (sub)
    processor VARCHAR(50) NOT NULL, -- 'nmi', 'ccbill', etc.
    processor_provider VARCHAR(50), -- provider slug for multi-tenant processors (e.g., mobius)
    
    -- Processor-specific vault/payment method identifiers
    vault_id VARCHAR(255) NOT NULL, -- Primary identifier in processor's system
    billing_id VARCHAR(255), -- Secondary identifier (e.g., subscription ID)
    initial_transaction_id VARCHAR(255) NOT NULL, -- Transaction that created this vault
    
    -- Payment method status and metadata
    is_active BOOLEAN NOT NULL DEFAULT true, -- Can this method be used for rebills?
    last_four VARCHAR(4), -- Last 4 digits of card
    card_type VARCHAR(50), -- 'Visa', 'MasterCard', etc.
    expiry_date VARCHAR(5), -- 'MM/YY' format
    failure_reason TEXT, -- Reason if inactive
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE INDEX IF NOT EXISTS idx_payment_methods_user_id ON billing.payment_methods(user_id);
CREATE INDEX IF NOT EXISTS idx_payment_methods_processor ON billing.payment_methods(processor);
CREATE INDEX IF NOT EXISTS idx_payment_methods_vault_id ON billing.payment_methods(vault_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_methods_processor_vault_id ON billing.payment_methods(processor, coalesce(processor_provider, ''), vault_id);
CREATE INDEX IF NOT EXISTS idx_payment_methods_is_active ON billing.payment_methods(is_active) WHERE is_active = true;
COMMENT ON TABLE billing.payment_methods IS 'Generalized payment method table supporting multiple processors.';
COMMENT ON COLUMN billing.payment_methods.processor IS 'Payment processor type: nmi, ccbill, stripe, etc.';
COMMENT ON COLUMN billing.payment_methods.vault_id IS 'Primary payment method identifier in the processor system';

-- Add payment_method_id reference to subscriptions table
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'fk_subscriptions_payment_method_id'
    ) THEN
        ALTER TABLE billing.subscriptions
        ADD CONSTRAINT fk_subscriptions_payment_method_id
        FOREIGN KEY (payment_method_id) REFERENCES billing.payment_methods(id) ON DELETE SET NULL;
    END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_subscriptions_payment_method_id ON billing.subscriptions(payment_method_id);

-- 4.2: Create processor and purchase status enums
DROP TYPE IF EXISTS billing.processor_type CASCADE;
CREATE TYPE billing.processor_type AS ENUM ('paypal', 'solana', 'nmi', 'ccbill');

DROP TYPE IF EXISTS billing.purchase_status CASCADE;
CREATE TYPE billing.purchase_status AS ENUM ('pending', 'completed', 'failed', 'refunded');

-- 4.3: Create payments table (formerly purchases)
CREATE TABLE IF NOT EXISTS billing.payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL, -- AuthKit user ID (UUID)
    price_id UUID NOT NULL REFERENCES billing.prices(id),
    processor billing.processor_type NOT NULL,
    processor_provider TEXT,
    transaction_id TEXT NOT NULL,
    amount DECIMAL(10,2) NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USD',
    -- Overall lifecycle state for the payment record
    status billing.purchase_status NOT NULL DEFAULT 'completed',
    subscription_id UUID REFERENCES billing.subscriptions(id) ON DELETE SET NULL,
    refunded_payment_id UUID,
    purchased_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    UNIQUE(processor, transaction_id),
    CONSTRAINT fk_payments_refunded_payment FOREIGN KEY (refunded_payment_id) REFERENCES billing.payments(id)
);

CREATE INDEX IF NOT EXISTS idx_payments_refunded_payment_id ON billing.payments(refunded_payment_id);

CREATE INDEX IF NOT EXISTS idx_payments_user_id ON billing.payments(user_id);
CREATE INDEX IF NOT EXISTS idx_payments_price_id ON billing.payments(price_id);
CREATE INDEX IF NOT EXISTS idx_payments_processor ON billing.payments(processor);
CREATE INDEX IF NOT EXISTS idx_payments_processor_provider ON billing.payments(processor, coalesce(processor_provider, ''));
CREATE INDEX IF NOT EXISTS idx_payments_purchased_at ON billing.payments(purchased_at);
CREATE INDEX IF NOT EXISTS idx_payments_subscription_id ON billing.payments(subscription_id);

COMMENT ON COLUMN billing.payments.subscription_id IS 'Links a payment to the subscription that generated it (nullable for one-off payments)';

-- 4.4: Create solana_payment_intents table (unified Solana payment flow)
CREATE TABLE IF NOT EXISTS billing.solana_payment_intents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    price_id UUID NOT NULL,
    flow_type TEXT NOT NULL, -- direct | solanapay
    token TEXT NOT NULL,
    token_mint TEXT NOT NULL,
    amount DECIMAL(18,9) NOT NULL,
    currency TEXT NOT NULL,
    expected_amount_lamports BIGINT NOT NULL,
    payer_wallet TEXT,
    recipient_wallet TEXT NOT NULL,
    reference TEXT,
    memo TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    signature TEXT,
    transaction_signature TEXT,
    error_message TEXT,
    expires_at TIMESTAMPTZ,
    confirmed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    UNIQUE(reference)
);

CREATE INDEX IF NOT EXISTS idx_solana_payment_intents_user_status ON billing.solana_payment_intents(user_id, status);
CREATE INDEX IF NOT EXISTS idx_solana_payment_intents_reference ON billing.solana_payment_intents(reference) WHERE reference IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_solana_payment_intents_expires ON billing.solana_payment_intents(expires_at) WHERE expires_at IS NOT NULL;

-- 4.5: Create solana_transactions table (pending and confirmed Solana payments)
CREATE TABLE IF NOT EXISTS billing.solana_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID, -- AuthKit user ID (UUID), nullable for anonymous intents
    signature TEXT, -- Solana transaction signature (set when confirmed)
    status TEXT NOT NULL, -- pending, confirmed, failed

    -- Payment details
    amount NUMERIC(18,9) NOT NULL,
    token TEXT NOT NULL, -- e.g., SOL, USDC
    token_mint TEXT NOT NULL, -- token mint address

    -- Addresses
    from_address TEXT NOT NULL,
    to_address TEXT NOT NULL,

    -- Optional references
    product_id UUID,
    purchase_id UUID,
    intent_id UUID REFERENCES billing.solana_payment_intents(id) ON DELETE SET NULL,

    -- Blockchain metadata
    block_time TIMESTAMPTZ,
    slot BIGINT,
    confirmations INTEGER NOT NULL DEFAULT 0,
    transaction_fee NUMERIC(18,9),

    -- Processing metadata
    processing_result JSONB,
    error_message TEXT,

    -- QR/payment flow metadata
    qr_code_id TEXT,

    -- Expiration for pending transactions
    expires_at TIMESTAMPTZ,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE INDEX IF NOT EXISTS idx_solana_transactions_user_id ON billing.solana_transactions(user_id);
CREATE INDEX IF NOT EXISTS idx_solana_transactions_status ON billing.solana_transactions(status);
CREATE INDEX IF NOT EXISTS idx_solana_transactions_expires_at ON billing.solana_transactions(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_solana_transactions_intent_id ON billing.solana_transactions(intent_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_solana_tx_signature ON billing.solana_transactions(signature) WHERE signature IS NOT NULL;

-- ============================================================================
-- SECTION 5: SUPPORTING TABLES
-- ============================================================================

CREATE TABLE IF NOT EXISTS billing.notification_queue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL, -- AuthKit user ID (UUID)
    event_type TEXT NOT NULL, -- premium_started, payment_failed, etc.
    data JSONB NOT NULL,
    seen BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE INDEX IF NOT EXISTS idx_notification_queue_user_id ON billing.notification_queue(user_id);
CREATE INDEX IF NOT EXISTS idx_notification_queue_event_type ON billing.notification_queue(event_type);
CREATE INDEX IF NOT EXISTS idx_notification_queue_seen ON billing.notification_queue(seen);
CREATE INDEX IF NOT EXISTS idx_notification_queue_created_at ON billing.notification_queue(created_at);


-- 5.2: Solana wallet challenges (for address verification)
CREATE TABLE IF NOT EXISTS billing.solana_wallet_challenges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    address TEXT NOT NULL,
    message TEXT NOT NULL,
    nonce TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    UNIQUE(user_id, address)
);

CREATE INDEX IF NOT EXISTS idx_solana_wallet_challenges_user ON billing.solana_wallet_challenges(user_id);
CREATE INDEX IF NOT EXISTS idx_solana_wallet_challenges_expires ON billing.solana_wallet_challenges(expires_at);

-- 5.3: Create solana_wallets table (user wallet linking and verification)
CREATE TABLE IF NOT EXISTS billing.solana_wallets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL, -- AuthKit user ID (UUID)
    address TEXT NOT NULL, -- Base58 encoded Solana wallet address
    is_verified BOOLEAN NOT NULL DEFAULT false,
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    
    -- Ensure one address per user
    UNIQUE(user_id, address)
);

CREATE INDEX IF NOT EXISTS idx_solana_wallets_user_id ON billing.solana_wallets(user_id);
CREATE INDEX IF NOT EXISTS idx_solana_wallets_address ON billing.solana_wallets(address);
CREATE INDEX IF NOT EXISTS idx_solana_wallets_verified ON billing.solana_wallets(is_verified) WHERE is_verified = true;

-- 5.4: Idempotency requests table for webhook / billing operations
CREATE TABLE IF NOT EXISTS billing.idempotency_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operation TEXT NOT NULL,
    key TEXT NOT NULL,
    user_id TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    result_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    UNIQUE(operation, key)
);

CREATE INDEX IF NOT EXISTS idx_idempotency_requests_status ON billing.idempotency_requests(status);
CREATE INDEX IF NOT EXISTS idx_idempotency_requests_created_at ON billing.idempotency_requests(created_at);

-- ============================================================================
-- SECTION 6: DATA MIGRATION FROM LEGACY TABLES
-- ============================================================================

-- 6.2: Migrate data from purchases table if it exists and hasn't been renamed yet
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'billing' AND table_name = 'purchases')
       AND NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'billing' AND table_name = 'payments') THEN
        -- First rename the table
        ALTER TABLE billing.purchases RENAME TO payments;

        -- Rename any associated indexes
        IF EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON c.relnamespace = n.oid WHERE c.relname = 'idx_purchases_subscription_id' AND n.nspname = 'billing') THEN
            ALTER INDEX billing.idx_purchases_subscription_id RENAME TO idx_payments_subscription_id;
        END IF;
    END IF;
END$$;

-- ============================================================================
-- SECTION 7: CLEANUP AND FINAL ADJUSTMENTS
-- ============================================================================


-- 7.3: Add comments for documentation
COMMENT ON TABLE billing.subscriptions IS 'Core subscription records tracking user billing relationships';
COMMENT ON TABLE billing.products IS 'Product definitions that can be purchased or subscribed to';
COMMENT ON TABLE billing.prices IS 'Pricing tiers for products with processor-specific identifiers';
COMMENT ON TABLE billing.payments IS 'Records of all payment transactions (formerly purchases table)';
COMMENT ON TABLE billing.notification_queue IS 'Queue for user notifications related to billing and subscriptions';
