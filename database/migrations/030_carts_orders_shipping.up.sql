
-- CraftBridge | Member 04: Cart, Orders & Delivery
-- Migration 030 - Run after identity and catalog migrations.
-- All monetary values are integer USD cents.

CREATE TABLE carts (
    id BIGSERIAL PRIMARY KEY,
    buyer_id BIGINT NOT NULL UNIQUE REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE cart_items (
    id BIGSERIAL PRIMARY KEY,
    cart_id BIGINT NOT NULL REFERENCES carts(id)
        ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id),
    quantity INTEGER NOT NULL CHECK (quantity BETWEEN 1 AND 99),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (cart_id, product_id)
);

CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    buyer_id BIGINT NOT NULL REFERENCES users(id),
    address_id BIGINT NOT NULL REFERENCES addresses(id),
    address_snapshot JSONB NOT NULL,
    idempotency_key VARCHAR(100) NOT NULL,
    subtotal_cents BIGINT NOT NULL DEFAULT 0
        CHECK (subtotal_cents >= 0),
    shipping_total_cents BIGINT NOT NULL DEFAULT 0
        CHECK (shipping_total_cents >= 0),
    grand_total_cents BIGINT NOT NULL DEFAULT 0
        CHECK (grand_total_cents >= 0),
    currency CHAR(3) NOT NULL DEFAULT 'USD'
        CHECK (currency = 'USD'),
    payment_status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (payment_status IN ('pending','paid','failed')),
    status VARCHAR(25) NOT NULL DEFAULT 'pending_payment'
        CHECK (status IN (
            'pending_payment','processing',
            'completed','cancelled','expired'
        )),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (buyer_id, idempotency_key),
    CHECK (grand_total_cents =
           subtotal_cents + shipping_total_cents)
);

CREATE TABLE seller_orders (
    id BIGSERIAL PRIMARY KEY,
    order_id BIGINT NOT NULL REFERENCES orders(id),
    shop_id BIGINT NOT NULL REFERENCES shops(id),
    subtotal_cents BIGINT NOT NULL CHECK (subtotal_cents >= 0),
    shipping_cents BIGINT NOT NULL DEFAULT 0
        CHECK (shipping_cents >= 0),
    status VARCHAR(25) NOT NULL DEFAULT 'pending_payment'
        CHECK (status IN (
            'pending_payment','processing',
            'shipped','delivered','cancelled'
        )),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (order_id, shop_id)
);

CREATE TABLE order_items (
    id BIGSERIAL PRIMARY KEY,
    seller_order_id BIGINT NOT NULL REFERENCES seller_orders(id),
    product_id BIGINT NOT NULL REFERENCES products(id),
    product_title_snapshot VARCHAR(200) NOT NULL,
    unit_price_cents BIGINT NOT NULL CHECK (unit_price_cents > 0),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    line_total_cents BIGINT NOT NULL CHECK (line_total_cents >= 0),
    CHECK (line_total_cents = unit_price_cents * quantity)
);

CREATE TABLE order_status_history (
    id BIGSERIAL PRIMARY KEY,
    order_id BIGINT NOT NULL REFERENCES orders(id),
    from_status VARCHAR(25),
    to_status VARCHAR(25) NOT NULL,
    actor_user_id BIGINT REFERENCES users(id),
    reason VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE stock_reservations (
    id BIGSERIAL PRIMARY KEY,
    order_item_id BIGINT NOT NULL UNIQUE REFERENCES order_items(id),
    product_id BIGINT NOT NULL REFERENCES products(id),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    status VARCHAR(15) NOT NULL DEFAULT 'reserved'
        CHECK (status IN ('reserved','committed','released')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE shipments (
    id BIGSERIAL PRIMARY KEY,
    seller_order_id BIGINT NOT NULL UNIQUE REFERENCES seller_orders(id),
    carrier VARCHAR(100),
    tracking_number VARCHAR(150),
    status VARCHAR(20) NOT NULL DEFAULT 'processing'
        CHECK (status IN ('processing','shipped','delivered')),
    shipped_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE shipment_events (
    id BIGSERIAL PRIMARY KEY,
    shipment_id BIGINT NOT NULL REFERENCES shipments(id),
    status VARCHAR(20) NOT NULL,
    note VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_cart_items_product ON cart_items(product_id);
CREATE INDEX idx_orders_buyer_date ON orders(buyer_id, created_at DESC);
CREATE INDEX idx_orders_expiry ON orders(expires_at)
    WHERE status = 'pending_payment';
CREATE INDEX idx_seller_orders_shop_status
    ON seller_orders(shop_id, status);
CREATE INDEX idx_order_items_product ON order_items(product_id);
CREATE INDEX idx_stock_reservations_product
    ON stock_reservations(product_id);
CREATE INDEX idx_shipment_events_shipment
    ON shipment_events(shipment_id, created_at);
