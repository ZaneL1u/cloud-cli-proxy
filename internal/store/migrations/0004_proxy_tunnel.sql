-- Extend egress_ips with tunnel type differentiation and proxy configuration.
ALTER TABLE egress_ips
    ADD COLUMN tunnel_type TEXT NOT NULL DEFAULT 'wireguard';

ALTER TABLE egress_ips
    ADD COLUMN proxy_config TEXT;
