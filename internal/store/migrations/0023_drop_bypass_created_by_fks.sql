-- 0023_drop_bypass_created_by_fks.sql
-- 移除 host_bypass_snapshots.created_by 和 host_bypass_audit_log.actor_id 的外键约束。
--
-- 原因：这两个是审计字段，当 JWT 中的 user_id 在 users 表中不存在时（数据库重建、
-- 用户被删后重建等场景），FK 约束会导致整个 INSERT 失败，业务请求返回 500。
-- 审计字段不应阻塞业务写入 — 用户 ID 只是记录"谁做了这个操作"，即使该用户已不存在，
-- 保留原始 ID 对审计追溯仍然有价值。
--
-- SQLite 不支持 ALTER TABLE DROP CONSTRAINT，使用重建表模式。

PRAGMA foreign_keys=OFF;

-- Rebuild host_bypass_snapshots without created_by FK
CREATE TABLE host_bypass_snapshots_new (
    id                      TEXT PRIMARY KEY,
    host_id                 TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    version                 INTEGER NOT NULL,
    config_hash             TEXT NOT NULL,
    whitelist_cidrs_json    TEXT NOT NULL DEFAULT '{"version":3,"rules":[]}',
    whitelist_domains_json  TEXT NOT NULL DEFAULT '{"version":3,"rules":[]}',
    applied_status          TEXT NOT NULL DEFAULT 'pending'
                            CHECK (applied_status IN ('pending','applied','failed','rolled_back')),
    source                  TEXT NOT NULL DEFAULT 'apply',
    created_by              TEXT,
    created_at              TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    UNIQUE (host_id, config_hash)
);

INSERT INTO host_bypass_snapshots_new
    SELECT * FROM host_bypass_snapshots;

DROP TABLE host_bypass_snapshots;
ALTER TABLE host_bypass_snapshots_new RENAME TO host_bypass_snapshots;

CREATE INDEX IF NOT EXISTS idx_bypass_snapshots_host_version ON host_bypass_snapshots(host_id, version DESC);

-- Rebuild host_bypass_audit_log without actor_id FK
CREATE TABLE host_bypass_audit_log_new (
    id           TEXT PRIMARY KEY,
    actor_id     TEXT,
    actor_ip     TEXT,
    action       TEXT NOT NULL,
    target_kind  TEXT NOT NULL,
    target_id    TEXT,
    before       TEXT,
    after        TEXT,
    note         TEXT,
    created_at   TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

INSERT INTO host_bypass_audit_log_new
    SELECT * FROM host_bypass_audit_log;

DROP TABLE host_bypass_audit_log;
ALTER TABLE host_bypass_audit_log_new RENAME TO host_bypass_audit_log;

CREATE INDEX IF NOT EXISTS idx_bypass_audit_target  ON host_bypass_audit_log(target_kind, target_id);
CREATE INDEX IF NOT EXISTS idx_bypass_audit_created ON host_bypass_audit_log(created_at DESC);

PRAGMA foreign_keys=ON;
