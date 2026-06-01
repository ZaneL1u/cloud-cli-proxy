-- 将 hosts 表的资源限制列改为 nullable。
-- NULL = 无限制（Docker 层面不传资源参数）
-- 正值 = 具体限制
-- 现有数据保留不变（已有主机的 4096MB / 2.0 CPU / 20GB 是显式限制）
-- 默认值逻辑移至 API 层（见 Phase 57 D-01 / D-02）
--
-- SQLite 不支持 ALTER COLUMN，使用重建表模式。

PRAGMA foreign_keys=OFF;

CREATE TABLE hosts_new (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',
    template_image_ref TEXT NOT NULL,
    home_volume_name TEXT NOT NULL,
    slot_key TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    UNIQUE (user_id, slot_key),
    timezone TEXT NOT NULL DEFAULT 'America/Los_Angeles',
    hostname TEXT NOT NULL DEFAULT '',
    short_id TEXT UNIQUE,
    memory_limit_mb INTEGER,
    cpu_limit REAL,
    disk_limit_gb INTEGER,
    host_mounts TEXT NOT NULL DEFAULT '[]'
);

INSERT INTO hosts_new
    (id, user_id, status, template_image_ref, home_volume_name, slot_key,
     created_at, updated_at, timezone, hostname, short_id,
     memory_limit_mb, cpu_limit, disk_limit_gb, host_mounts)
SELECT
    id, user_id, status, template_image_ref, home_volume_name, slot_key,
    created_at, updated_at, timezone, hostname, short_id,
    memory_limit_mb, cpu_limit, disk_limit_gb, host_mounts
FROM hosts;

DROP TABLE hosts;
ALTER TABLE hosts_new RENAME TO hosts;

-- Recreate indexes that reference hosts
CREATE INDEX IF NOT EXISTS idx_hosts_user_id ON hosts (user_id);
CREATE INDEX IF NOT EXISTS idx_hosts_status_running ON hosts (status) WHERE status = 'running';
CREATE UNIQUE INDEX IF NOT EXISTS idx_hosts_user_active
  ON hosts (user_id)
  WHERE status NOT IN ('deleted', 'archived');

PRAGMA foreign_keys=ON;
