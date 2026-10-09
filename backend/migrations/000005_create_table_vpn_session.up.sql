-- One row per VPN connection. The device is deleted when the user disconnects
-- (to free its IP), so device_id is kept without a foreign key and the session
-- stays as connection history.
CREATE TABLE IF NOT EXISTS "vpn_session" (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID        NOT NULL REFERENCES "user" (id),
    server_id        UUID        NOT NULL REFERENCES "server" (id),
    device_id        UUID        NOT NULL,
    assigned_ip      VARCHAR(45) NOT NULL,
    connected_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    disconnected_at  TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_vpn_session_user_id ON "vpn_session" (user_id, connected_at DESC);
-- A device has at most one open session.
CREATE UNIQUE INDEX IF NOT EXISTS idx_vpn_session_open_device
    ON "vpn_session" (device_id) WHERE disconnected_at IS NULL;
