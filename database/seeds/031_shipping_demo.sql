
-- Demonstration shipping prices for existing shops.
-- USD 7.00 base + USD 1.00 per additional unit.
-- Local development only, not an approved production tariff.

INSERT INTO seller_shipping_rates (
    shop_id,
    destination_country,
    base_fee_cents,
    additional_item_fee_cents
)
SELECT
    id,
    'US',
    700,
    100
FROM shops
WHERE status = 'active'
ON CONFLICT (shop_id, destination_country)
DO NOTHING;
