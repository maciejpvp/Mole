CREATE INDEX users_username_pattern_idx ON users (username text_pattern_ops);
CREATE INDEX users_email_pattern_idx ON users (email text_pattern_ops);
CREATE INDEX users_minutes_sort_idx ON users (monthly_minutes_used DESC, id ASC);
CREATE INDEX users_transfer_sort_idx ON users (monthly_transfer_bytes_used DESC, id ASC);
CREATE INDEX users_created_sort_idx ON users (created_at DESC, id ASC);
