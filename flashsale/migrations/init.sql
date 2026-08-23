-- Flash Sale schema.
--
-- Applied automatically by the postgres container on first start, and safe to
-- re-run against an existing database: every statement is idempotent.

-- gen_random_uuid() is built in from PostgreSQL 13, but pgcrypto keeps this
-- schema usable on older servers too.
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role          VARCHAR(50) NOT NULL DEFAULT 'user',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS products (
    id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Unique names let the seed script be re-run without creating duplicates.
    name  VARCHAR(255) NOT NULL UNIQUE,
    price INT NOT NULL CHECK (price >= 0),
    -- The stock floor is what stops the durable record from overselling, even
    -- if the in-memory counter and the database ever diverge.
    stock INT NOT NULL CHECK (stock >= 0)
);

CREATE TABLE IF NOT EXISTS orders (
    id         UUID PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id),
    product_id UUID NOT NULL REFERENCES products(id),
    qty        INT NOT NULL CHECK (qty > 0),
    status     VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders(user_id);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);

-- Constraints added after the original schema. ADD CONSTRAINT has no
-- IF NOT EXISTS form, so each one is guarded by a catalogue lookup to keep this
-- file re-runnable against databases created before they existed.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'products_name_key'
    ) THEN
        ALTER TABLE products ADD CONSTRAINT products_name_key UNIQUE (name);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'orders_qty_check'
    ) THEN
        ALTER TABLE orders ADD CONSTRAINT orders_qty_check CHECK (qty > 0);
    END IF;

    -- Restricting status to the states the services actually produce turns a
    -- typo in application code into a failed write instead of a silently
    -- unreadable order.
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'orders_status_check'
    ) THEN
        ALTER TABLE orders ADD CONSTRAINT orders_status_check
            CHECK (status IN ('PENDING', 'SUCCESS', 'FAILED', 'CANCELLED'));
    END IF;
END
$$;
