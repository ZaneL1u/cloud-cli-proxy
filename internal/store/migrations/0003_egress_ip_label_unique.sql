-- 0003_egress_ip_label_unique.sql
-- 将唯一约束从 ip_address 迁移到 label：ip_address 去 NOT NULL/UNIQUE（可空可重复），
-- label 加 UNIQUE。SQLite 无法 DROP 列级约束，须表重建。
-- egress_ips 被 host_egress_bindings 以 ON DELETE RESTRICT 引用，连接 foreign_keys=ON，
-- migrator 单事务内无法关 FK；实测 defer_foreign_keys 在 COMMIT 仍报 FK failed(787)，不可用。
-- 故采用子表重建法：先暂存并 DROP 子表 → 重建 egress_ips → 按 0001 原定义恢复子表与索引。

-- 1) 暂存 host_egress_bindings 数据（临时表无约束）
CREATE TABLE _host_egress_bindings_backup AS SELECT * FROM host_egress_bindings;

-- 2) DROP 子表，解除对 egress_ips 的 RESTRICT 引用
DROP TABLE host_egress_bindings;

-- 3) 重建 egress_ips：ip_address 去 NOT NULL/UNIQUE，label 加 UNIQUE
CREATE TABLE egress_ips_new (
    id                  TEXT PRIMARY KEY,
    label               TEXT NOT NULL UNIQUE,
    ip_address          TEXT,
    provider            TEXT NOT NULL DEFAULT 'manual',
    status              TEXT NOT NULL DEFAULT 'available',
    created_at          DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at          DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    proxy_config        TEXT,
    detected_ip_address TEXT
);
INSERT INTO egress_ips_new
    (id, label, ip_address, provider, status, created_at, updated_at, proxy_config, detected_ip_address)
SELECT
    id, label, ip_address, provider, status, created_at, updated_at, proxy_config, detected_ip_address
FROM egress_ips;
DROP TABLE egress_ips;
ALTER TABLE egress_ips_new RENAME TO egress_ips;

-- 4) 按 0001 原定义恢复 host_egress_bindings（含两个 FK 与 UNIQUE）
CREATE TABLE host_egress_bindings (
    id          TEXT PRIMARY KEY,
    host_id     TEXT NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
    egress_ip_id TEXT NOT NULL REFERENCES egress_ips (id) ON DELETE RESTRICT,
    created_at  DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    UNIQUE (host_id, egress_ip_id)
);
INSERT INTO host_egress_bindings (id, host_id, egress_ip_id, created_at)
    SELECT id, host_id, egress_ip_id, created_at FROM _host_egress_bindings_backup;

-- 5) 恢复索引（随子表 DROP 消失，必须重建）
CREATE INDEX IF NOT EXISTS idx_host_egress_bindings_host_id ON host_egress_bindings (host_id);

-- 6) 清理暂存表
DROP TABLE _host_egress_bindings_backup;
