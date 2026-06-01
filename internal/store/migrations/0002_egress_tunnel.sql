-- Extend egress_ips with WireGuard tunnel configuration fields.
ALTER TABLE egress_ips ADD COLUMN wg_endpoint TEXT;
ALTER TABLE egress_ips ADD COLUMN wg_public_key TEXT;
ALTER TABLE egress_ips ADD COLUMN wg_preshared_key TEXT;
ALTER TABLE egress_ips ADD COLUMN wg_allowed_ips TEXT NOT NULL DEFAULT '0.0.0.0/0';
ALTER TABLE egress_ips ADD COLUMN wg_dns_server TEXT;
ALTER TABLE egress_ips ADD COLUMN wg_peer_address TEXT;

-- Extend hosts with per-host WireGuard key pair.
ALTER TABLE hosts ADD COLUMN wg_private_key TEXT;
ALTER TABLE hosts ADD COLUMN wg_public_key TEXT;
