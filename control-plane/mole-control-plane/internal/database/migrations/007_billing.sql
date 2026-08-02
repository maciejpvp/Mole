INSERT INTO plans (name, max_active_tunnels, monthly_minutes, monthly_transfer_bytes)
VALUES ('pending', 0, 0, 0)
ON CONFLICT (name) DO NOTHING;

CREATE TABLE billing_customers (
    user_id TEXT PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    stripe_customer_id TEXT NOT NULL UNIQUE,
    stripe_payment_method_id TEXT,
    latest_setup_intent_id TEXT UNIQUE,
    card_verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE stripe_events (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER billing_customers_set_updated_at
BEFORE UPDATE ON billing_customers
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
