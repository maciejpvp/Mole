TRUNCATE TABLE users CASCADE;

ALTER TABLE users
    DROP COLUMN password_hash,
    DROP COLUMN failed_login_attempts,
    DROP COLUMN locked_until,
    ADD COLUMN google_subject TEXT NOT NULL UNIQUE;

CREATE TABLE auth_login_codes (
    code_hash BYTEA PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX auth_login_codes_expires_at_idx ON auth_login_codes (expires_at);
