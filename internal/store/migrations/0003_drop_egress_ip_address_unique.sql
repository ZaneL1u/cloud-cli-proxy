-- 0003_drop_egress_ip_address_unique.sql
-- ip_address 是可重复的检测结果，不是出口资源的唯一身份。
-- SQLite 无法直接移除列级 UNIQUE 约束，因此在保留绑定关系的前提下重建表。

CREATE TABLE _host_egress_bindings_backup AS
SELECT id, host_id, egress_ip_id, created_at
FROM host_egress_bindings;

DROP TABLE host_egress_bindings;

CREATE TABLE egress_ips_new (
    id                  TEXT PRIMARY KEY,
    label               TEXT NOT NULL,
    ip_address          TEXT NOT NULL,
    provider            TEXT NOT NULL DEFAULT 'manual',
    status              TEXT NOT NULL DEFAULT 'available',
    created_at          DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at          DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    proxy_config        TEXT,
    detected_ip_address TEXT
);

INSERT INTO egress_ips_new (
    id, label, ip_address, provider, status,
    created_at, updated_at, proxy_config, detected_ip_address
)
SELECT
    id, label, ip_address, provider, status,
    created_at, updated_at, proxy_config, detected_ip_address
FROM egress_ips;

DROP TABLE egress_ips;
ALTER TABLE egress_ips_new RENAME TO egress_ips;

CREATE TABLE host_egress_bindings (
    id           TEXT PRIMARY KEY,
    host_id      TEXT NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
    egress_ip_id TEXT NOT NULL REFERENCES egress_ips (id) ON DELETE RESTRICT,
    created_at   DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    UNIQUE (host_id, egress_ip_id)
);

INSERT INTO host_egress_bindings (id, host_id, egress_ip_id, created_at)
SELECT id, host_id, egress_ip_id, created_at
FROM _host_egress_bindings_backup;

CREATE INDEX IF NOT EXISTS idx_host_egress_bindings_host_id
    ON host_egress_bindings (host_id);

DROP TABLE _host_egress_bindings_backup;
