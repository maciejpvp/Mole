CREATE TABLE plans (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    max_active_tunnels INTEGER,
    monthly_minutes BIGINT,
    monthly_transfer_bytes BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT plans_max_active_tunnels_nonnegative CHECK (max_active_tunnels IS NULL OR max_active_tunnels >= 0),
    CONSTRAINT plans_monthly_minutes_nonnegative CHECK (monthly_minutes IS NULL OR monthly_minutes >= 0),
    CONSTRAINT plans_monthly_transfer_bytes_nonnegative CHECK (monthly_transfer_bytes IS NULL OR monthly_transfer_bytes >= 0)
);

COMMENT ON COLUMN plans.max_active_tunnels IS 'Maximum concurrent tunnels. NULL means unlimited.';
COMMENT ON COLUMN plans.monthly_minutes IS 'Monthly tunnel-minute allowance. NULL means unlimited.';
COMMENT ON COLUMN plans.monthly_transfer_bytes IS 'Monthly data-transfer allowance in bytes. NULL means unlimited.';

INSERT INTO plans (name, max_active_tunnels, monthly_minutes, monthly_transfer_bytes)
VALUES
    ('free', 1, 60, 1073741824),
    ('premium', 3, 260, 10737418240),
    ('unlimited', NULL, NULL, NULL);

CREATE TABLE users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    plan_id BIGINT NOT NULL REFERENCES plans (id) ON DELETE RESTRICT,
    usage_period_started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    monthly_minutes_used BIGINT NOT NULL DEFAULT 0,
    monthly_transfer_bytes_used BIGINT NOT NULL DEFAULT 0,
    usage_limit_reached_at TIMESTAMPTZ,
    failed_login_attempts INTEGER NOT NULL DEFAULT 0,
    locked_until TIMESTAMPTZ,
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT users_monthly_minutes_used_nonnegative CHECK (monthly_minutes_used >= 0),
    CONSTRAINT users_monthly_transfer_bytes_used_nonnegative CHECK (monthly_transfer_bytes_used >= 0),
    CONSTRAINT users_failed_login_attempts_nonnegative CHECK (failed_login_attempts >= 0),
    CONSTRAINT users_username_format CHECK (username ~ '^[a-z0-9][a-z0-9_-]{2,31}$'),
    CONSTRAINT users_email_normalized CHECK (email = lower(email))
);

CREATE INDEX users_plan_id_idx ON users (plan_id);

COMMENT ON COLUMN users.id IS 'Stable identifier supplied by the authentication provider.';
COMMENT ON COLUMN users.usage_period_started_at IS 'Start of the billing/usage period represented by the monthly counters.';
COMMENT ON COLUMN users.usage_limit_reached_at IS 'When the current plan limit was first exceeded; NULL when within limits.';

CREATE TABLE sessions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMPTZ
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

CREATE TABLE tunnels (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    proto smallint NOT NULL DEFAULT 6, -- 6 = TCP, 17 = UDP
    outbound_port INTEGER NOT NULL,
    inbound_ip INET NOT NULL,
    inbound_port INTEGER NOT NULL,
    server_address TEXT NOT NULL,
    connection_token_hash BYTEA NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'inactive',
    started_at TIMESTAMPTZ,
    stopped_at TIMESTAMPTZ,
    current_period_minutes BIGINT NOT NULL DEFAULT 0,
    current_period_transfer_bytes BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT tunnels_outbound_port_valid CHECK (outbound_port BETWEEN 1 AND 65535),
    CONSTRAINT tunnels_inbound_port_valid CHECK (inbound_port BETWEEN 1 AND 65535),
    CONSTRAINT tunnels_status_valid CHECK (status IN ('inactive', 'active', 'stopped')),
    CONSTRAINT tunnels_proto_valid CHECK (proto IN (6, 17)),
    CONSTRAINT tunnels_current_period_minutes_nonnegative CHECK (current_period_minutes >= 0),
    CONSTRAINT tunnels_current_period_transfer_bytes_nonnegative CHECK (current_period_transfer_bytes >= 0)
);

CREATE INDEX tunnels_user_id_idx ON tunnels (user_id);
CREATE INDEX tunnels_user_id_status_idx ON tunnels (user_id, status);
CREATE UNIQUE INDEX tunnels_active_outbound_port_unique ON tunnels (outbound_port) WHERE status IN ('inactive', 'active');

COMMENT ON COLUMN tunnels.outbound_port IS 'Public port exposed by the control plane.';
COMMENT ON COLUMN tunnels.inbound_ip IS 'IP address of the service receiving tunneled traffic.';
COMMENT ON COLUMN tunnels.inbound_port IS 'Port of the service receiving tunneled traffic.';
COMMENT ON COLUMN tunnels.status IS 'inactive until the client connects, active while connected, stopped after manual or quota enforcement shutdown.';

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER plans_set_updated_at
BEFORE UPDATE ON plans
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER users_set_updated_at
BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER tunnels_set_updated_at
BEFORE UPDATE ON tunnels
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
