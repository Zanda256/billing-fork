-- 00002_seed_billings_tables.sql
-- Seed data for billing tables - products, prices, and default subscription plans

-- Explicitly set schema to ensure all objects are created in the correct place
-- Set timeouts to prevent hanging migrations
SET lock_timeout = '10s';
SET statement_timeout = '300s';

-- ============================================================================
-- SECTION 0: CLEANUP LEGACY DATA
-- ============================================================================

-- Update any NULL processor_subscription_id values with defaults
UPDATE billing.subscriptions
SET processor_subscription_id = 'LEGACY-' || id::text
WHERE processor_subscription_id IS NULL OR processor_subscription_id = '';

-- Set default started_at for subscriptions that don't have it
UPDATE billing.subscriptions
SET started_at = created_at
WHERE started_at IS NULL;

-- SECTION 2: SEED PRODUCTS CATALOG
-- ============================================================================

-- Seed product catalog and pricing with deterministic IDs to match frontend fixtures
DO $$
DECLARE
    basic_product_id CONSTANT UUID := 'c454b402-80e7-4f84-924f-99ffaa82d002';
    premium_product_id CONSTANT UUID := 'bc5fe9a7-b8cf-4a61-b618-886b4c7b6faa';
    basic_price_id CONSTANT UUID := '1028cc14-cdcb-4be7-92b1-8d434f220d3f';
    premium_price_id CONSTANT UUID := '99854e03-4f2e-421d-83a5-bf9d46223f01';
BEGIN
    -- Insert products with fixed IDs (upsert to keep IDs aligned on re-runs)
    INSERT INTO billing.products (id, slug, display_name, description, entitlements_spec)
    VALUES
        (basic_product_id, 'basic_membership', 'Basic Membership', 'Essential access to the standard catalog and community features.', jsonb_build_object('basic', null)),
        (premium_product_id, 'premium_membership', 'Premium Membership', 'Everything in Basic plus premium catalog access and enhanced perks.', jsonb_build_object('basic', null, 'premium', null))
    ON CONFLICT (slug) DO UPDATE SET
        id = EXCLUDED.id,
        display_name = EXCLUDED.display_name,
        description = EXCLUDED.description,
        entitlements_spec = EXCLUDED.entitlements_spec,
        updated_at = current_timestamp;

    -- Insert pricing tiers with fixed IDs linked to the deterministic product IDs
    INSERT INTO billing.prices (id, product_id, display_name, amount, currency, billing_cycle_days, ccbill_price_id, nmi_plan_id, nmi_provider, is_active)
    VALUES
        (basic_price_id, basic_product_id, 'Basic Monthly', 4.99, 'USD', 30, '681cb38f-afb9-4665-931f-2b896072178a', 'basic_monthly', 'mobius', true),
        (premium_price_id, premium_product_id, 'Premium Monthly', 9.99, 'USD', 30, '681cb38f-afb9-4665-931f-2b896072178a', 'premium_monthly', 'mobius', true)
    ON CONFLICT (product_id, amount, currency, billing_cycle_days) DO UPDATE SET
        id = EXCLUDED.id,
        display_name = EXCLUDED.display_name,
        is_active = EXCLUDED.is_active,
        ccbill_price_id = EXCLUDED.ccbill_price_id,
        nmi_plan_id = EXCLUDED.nmi_plan_id,
        nmi_provider = EXCLUDED.nmi_provider,
        updated_at = current_timestamp;
END$$;

-- Add helpful comments for operators
COMMENT ON TABLE billing.products IS 'Product catalog - modify these via your application, not directly in billing service';
COMMENT ON TABLE billing.prices IS 'Pricing tiers with processor integration - modify these via your application for new campaigns';
COMMENT ON COLUMN billing.prices.is_active IS 'Set to false to disable pricing tier without deleting (useful for campaigns)';
COMMENT ON COLUMN billing.prices.ccbill_price_id IS 'CCBill FlexForm price identifier - update when creating new CCBill products';
COMMENT ON COLUMN billing.prices.nmi_plan_id IS 'NMI plan identifier - update when creating new NMI plans';
COMMENT ON COLUMN billing.prices.nmi_provider IS 'NMI provider slug (e.g., mobius) for multi-tenant gateways';
