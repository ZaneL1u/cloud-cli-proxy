-- Remove WireGuard-related columns from egress_ips and hosts.
-- WireGuard tunnel support has been removed; only proxy tunnels are supported.

ALTER TABLE egress_ips DROP COLUMN IF EXISTS wg_endpoint;
ALTER TABLE egress_ips DROP COLUMN IF EXISTS wg_public_key;
ALTER TABLE egress_ips DROP COLUMN IF EXISTS wg_preshared_key;
ALTER TABLE egress_ips DROP COLUMN IF EXISTS wg_allowed_ips;
ALTER TABLE egress_ips DROP COLUMN IF EXISTS wg_dns_server;
ALTER TABLE egress_ips DROP COLUMN IF EXISTS wg_peer_address;

ALTER TABLE egress_ips DROP COLUMN IF EXISTS tunnel_type;

ALTER TABLE hosts DROP COLUMN IF EXISTS wg_private_key;
ALTER TABLE hosts DROP COLUMN IF EXISTS wg_public_key;
