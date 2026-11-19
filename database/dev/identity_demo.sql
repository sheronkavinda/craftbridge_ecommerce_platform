
-- CraftBridge isolated local development identities
-- Do NOT apply this file to the shared team database.
-- Adeesha owns the actual users/roles schema.

CREATE TABLE IF NOT EXISTS users (
    id BIGINT PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL
);

INSERT INTO users (id, email)
VALUES
    (1001, 'seller-demo@example.test'),
    (1002, 'buyer-demo@example.test'),
    (9001, 'admin-demo@example.test')
ON CONFLICT (id) DO NOTHING;
