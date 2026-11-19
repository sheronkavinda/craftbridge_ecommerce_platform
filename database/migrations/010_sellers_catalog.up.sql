
-- =====================================================
-- CraftBridge - Seller and Catalog Database
-- Member 02: Kanishka
-- Migration: 010_sellers_catalog.up.sql
-- =====================================================

-- Important:
-- Run identity migration 001 before this migration.
-- Assumes users(id) is BIGINT.

-- 1. SELLER APPLICATIONS

CREATE TABLE seller_applications (
    id BIGSERIAL PRIMARY KEY,

    user_id BIGINT NOT NULL UNIQUE
        REFERENCES users(id),

    store_name VARCHAR(120) NOT NULL,
    country_code CHAR(2) NOT NULL,
    description TEXT,

    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN (
            'pending',
            'approved',
            'rejected'
        )),

    decision_reason TEXT,

    reviewed_by BIGINT
        REFERENCES users(id),

    reviewed_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 2. SHOPS

CREATE TABLE shops (
    id BIGSERIAL PRIMARY KEY,

    owner_user_id BIGINT NOT NULL UNIQUE
        REFERENCES users(id),

    application_id BIGINT NOT NULL UNIQUE
        REFERENCES seller_applications(id),

    name VARCHAR(120) NOT NULL,

    slug VARCHAR(150) NOT NULL UNIQUE,

    description TEXT,

    country_code CHAR(2) NOT NULL,

    contact_email VARCHAR(255),

    profile_image_url TEXT,
    banner_image_url TEXT,

    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN (
            'active',
            'suspended'
        )),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 3. CATEGORIES

CREATE TABLE categories (
    id BIGSERIAL PRIMARY KEY,

    name VARCHAR(100) NOT NULL,
    slug VARCHAR(120) NOT NULL UNIQUE,
    description TEXT,

    is_active BOOLEAN NOT NULL DEFAULT TRUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 4. PRODUCTS

CREATE TABLE products (
    id BIGSERIAL PRIMARY KEY,

    shop_id BIGINT NOT NULL
        REFERENCES shops(id),

    category_id BIGINT NOT NULL
        REFERENCES categories(id),

    title VARCHAR(200) NOT NULL,
    description TEXT NOT NULL,

    price_cents BIGINT NOT NULL
        CHECK (price_cents > 0),

    currency CHAR(3) NOT NULL DEFAULT 'USD'
        CHECK (currency = 'USD'),

    status VARCHAR(20) NOT NULL DEFAULT 'draft'
        CHECK (status IN (
            'draft',
            'published',
            'archived'
        )),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_products_shop
    ON products(shop_id);

CREATE INDEX idx_products_category_status
    ON products(category_id, status);

-- 5. PRODUCT IMAGES

CREATE TABLE product_images (
    id BIGSERIAL PRIMARY KEY,

    product_id BIGINT NOT NULL
        REFERENCES products(id) ON DELETE CASCADE,

    image_url TEXT NOT NULL,

    alt_text TEXT,

    sort_order INTEGER NOT NULL DEFAULT 0
        CHECK (sort_order >= 0),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 6. INVENTORY

CREATE TABLE inventory (
    product_id BIGINT PRIMARY KEY
        REFERENCES products(id),

    available_qty INTEGER NOT NULL DEFAULT 0
        CHECK (available_qty >= 0),

    reserved_qty INTEGER NOT NULL DEFAULT 0
        CHECK (reserved_qty >= 0),

    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 7. INVENTORY MOVEMENTS

CREATE TABLE inventory_movements (
    id BIGSERIAL PRIMARY KEY,

    product_id BIGINT NOT NULL
        REFERENCES products(id),

    available_delta INTEGER NOT NULL DEFAULT 0,

    reserved_delta INTEGER NOT NULL DEFAULT 0,

    reason VARCHAR(100) NOT NULL,

    actor_user_id BIGINT
        REFERENCES users(id),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT valid_inventory_movement
        CHECK (
            available_delta <> 0
            OR reserved_delta <> 0
        )
);

CREATE INDEX idx_inventory_movements_product
    ON inventory_movements(product_id, created_at);
