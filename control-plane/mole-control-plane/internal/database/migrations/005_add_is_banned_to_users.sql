ALTER TABLE users
    ADD COLUMN is_banned BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN users.is_banned IS 'Whether the user is prevented from accessing their account.';
