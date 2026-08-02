ALTER TABLE billing_customers
ADD COLUMN stripe_mode TEXT NOT NULL DEFAULT 'test'
CHECK (stripe_mode IN ('test', 'live'));
