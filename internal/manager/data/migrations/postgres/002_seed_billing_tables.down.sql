-- Explicitly set schema to ensure all objects are created in the correct place

DELETE FROM prices
WHERE ccbill_price_id IN ('ccbill_basic_monthly', 'ccbill_premium_monthly');

DELETE FROM products
WHERE slug IN ('basic_membership', 'premium_membership');
