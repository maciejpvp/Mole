ALTER TABLE tunnels
    DROP CONSTRAINT IF EXISTS tunnels_status_valid;

ALTER TABLE tunnels
    ALTER COLUMN status SET DEFAULT 'inactive',
    ADD CONSTRAINT tunnels_status_valid CHECK (status IN ('inactive', 'active', 'stopped'));

DROP INDEX IF EXISTS tunnels_active_outbound_port_unique;

CREATE UNIQUE INDEX tunnels_active_outbound_port_unique
    ON tunnels (outbound_port)
    WHERE status IN ('inactive', 'active');

COMMENT ON COLUMN tunnels.status IS 'inactive until the client connects, active while connected, stopped after manual or quota enforcement shutdown.';
