-- +goose Up
CREATE TABLE users (
    id         BIGSERIAL PRIMARY KEY,
    login      VARCHAR(255) UNIQUE NOT NULL,
    password   BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE orders (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id),
    number      VARCHAR(255) UNIQUE NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'NEW',
    accrual     BIGINT NOT NULL DEFAULT 0,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_orders_user_id ON orders(user_id);
CREATE INDEX idx_orders_status ON orders(status);

CREATE TABLE withdrawals (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users(id),
    order_number VARCHAR(255) NOT NULL,
    amount       BIGINT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_withdrawals_user_id ON withdrawals(user_id);

-- +goose Down
DROP INDEX IF EXISTS idx_withdrawals_user_id;
DROP TABLE IF EXISTS withdrawals;
DROP INDEX IF EXISTS idx_orders_status;
DROP INDEX IF EXISTS idx_orders_user_id;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS users;
