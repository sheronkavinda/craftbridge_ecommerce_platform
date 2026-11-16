
-- CraftBridge Member 04
-- Shipping fee rules for multi-seller checkout.
-- Requires shops table from Kanishka's catalog migration.

CREATE TABLE seller_shipping_rates (
    id BIGSERIAL PRIMARY KEY,

    shop_id BIGINT NOT NULL
        REFERENCES shops(id),

    destination_country CHAR(2) NOT NULL,

    base_fee_cents BIGINT NOT NULL
        CHECK (base_fee_cents BETWEEN 0 AND 1000000),

    additional_item_fee_cents BIGINT NOT NULL
        CHECK (additional_item_fee_cents BETWEEN 0 AND 100000),

    is_active BOOLEAN NOT NULL DEFAULT TRUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (shop_id, destination_country)
);

CREATE INDEX idx_shipping_rates_country
    ON seller_shipping_rates(destination_country)
    WHERE is_active = TRUE;
