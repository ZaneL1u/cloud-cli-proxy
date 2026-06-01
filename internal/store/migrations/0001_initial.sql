-- 0001_initial.sql
-- Cloud CLI Proxy 初始 schema（SQLite）
-- 将所有增量迁移合并为单文件，直接反映最终表结构。

-- ============================================================
-- users
-- ============================================================
CREATE TABLE IF NOT EXISTS users (
    id             TEXT PRIMARY KEY,
    username       TEXT NOT NULL UNIQUE,
    password_hash  TEXT,
    status         TEXT NOT NULL DEFAULT 'active',
    created_at     DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at     DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    expires_at     DATETIME,
    short_id       TEXT UNIQUE,
    entry_password TEXT NOT NULL DEFAULT '',
    role           TEXT NOT NULL DEFAULT 'user',
    ssh_public_key  TEXT DEFAULT '',
    ssh_private_key TEXT DEFAULT '',
    ssh_key_type    TEXT DEFAULT ''
);

-- ============================================================
-- hosts
-- ============================================================
CREATE TABLE IF NOT EXISTS hosts (
    id                  TEXT PRIMARY KEY,
    user_id             TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status              TEXT NOT NULL DEFAULT 'pending',
    template_image_ref  TEXT NOT NULL,
    home_volume_name    TEXT NOT NULL,
    slot_key            TEXT NOT NULL,
    created_at          DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at          DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    timezone            TEXT NOT NULL DEFAULT 'America/Los_Angeles',
    hostname            TEXT NOT NULL DEFAULT '',
    short_id            TEXT UNIQUE,
    memory_limit_mb     INTEGER,
    cpu_limit           REAL,
    disk_limit_gb       INTEGER,
    host_mounts         TEXT NOT NULL DEFAULT '[]',
    UNIQUE (user_id, slot_key)
);

-- ============================================================
-- egress_ips
-- ============================================================
CREATE TABLE IF NOT EXISTS egress_ips (
    id                  TEXT PRIMARY KEY,
    label               TEXT NOT NULL,
    ip_address          TEXT NOT NULL UNIQUE,
    provider            TEXT NOT NULL DEFAULT 'manual',
    status              TEXT NOT NULL DEFAULT 'available',
    created_at          DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at          DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    proxy_config        TEXT,
    detected_ip_address TEXT
);

-- ============================================================
-- host_egress_bindings
-- ============================================================
CREATE TABLE IF NOT EXISTS host_egress_bindings (
    id          TEXT PRIMARY KEY,
    host_id     TEXT NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
    egress_ip_id TEXT NOT NULL REFERENCES egress_ips (id) ON DELETE RESTRICT,
    created_at  DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    UNIQUE (host_id, egress_ip_id)
);

-- ============================================================
-- tasks
-- ============================================================
CREATE TABLE IF NOT EXISTS tasks (
    id                 TEXT PRIMARY KEY,
    host_id            TEXT REFERENCES hosts (id) ON DELETE SET NULL,
    kind               TEXT NOT NULL,
    status             TEXT NOT NULL CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'canceled')),
    requested_by       TEXT NOT NULL,
    error_code         TEXT,
    error_message      TEXT,
    last_error_summary TEXT,
    created_at         DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at         DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    progress_percent   INTEGER NOT NULL DEFAULT 0,
    progress_message   TEXT NOT NULL DEFAULT ''
);

-- ============================================================
-- events
-- ============================================================
CREATE TABLE IF NOT EXISTS events (
    id         TEXT PRIMARY KEY,
    task_id    TEXT REFERENCES tasks (id) ON DELETE SET NULL,
    host_id    TEXT REFERENCES hosts (id) ON DELETE SET NULL,
    level      TEXT NOT NULL DEFAULT 'info',
    type       TEXT NOT NULL,
    message    TEXT NOT NULL,
    metadata   TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    user_id    TEXT REFERENCES users (id) ON DELETE SET NULL
);

-- ============================================================
-- claude_accounts
-- ============================================================
CREATE TABLE IF NOT EXISTS claude_accounts (
    id                      TEXT PRIMARY KEY,
    user_id                 TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    host_id                 TEXT REFERENCES hosts (id) ON DELETE SET NULL,
    email                   TEXT NOT NULL,
    display_name            TEXT NOT NULL DEFAULT '',
    status                  TEXT NOT NULL DEFAULT 'active',
    created_at              DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at              DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    persistent_volume_name  TEXT
);

-- ============================================================
-- ssh_keys
-- ============================================================
CREATE TABLE IF NOT EXISTS ssh_keys (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    purpose     TEXT NOT NULL CHECK (purpose IN ('inbound', 'outbound')),
    label       TEXT NOT NULL DEFAULT '',
    public_key  TEXT NOT NULL,
    private_key TEXT NOT NULL DEFAULT '',
    key_type    TEXT NOT NULL DEFAULT 'ed25519',
    fingerprint TEXT NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

-- ============================================================
-- host_bypass_presets
-- ============================================================
CREATE TABLE IF NOT EXISTS host_bypass_presets (
    id          TEXT PRIMARY KEY,
    slug        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    description TEXT,
    is_system   INTEGER NOT NULL DEFAULT 0,
    is_force_on INTEGER NOT NULL DEFAULT 0,
    is_active   INTEGER NOT NULL DEFAULT 1,
    rules       TEXT NOT NULL DEFAULT '[]',
    created_at  DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at  DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

-- ============================================================
-- host_bypass_rules
-- ============================================================
CREATE TABLE IF NOT EXISTS host_bypass_rules (
    id         TEXT PRIMARY KEY,
    scope      TEXT NOT NULL CHECK (scope IN ('global', 'host')),
    host_id    TEXT REFERENCES hosts (id) ON DELETE CASCADE,
    rule_type  TEXT NOT NULL CHECK (rule_type IN ('ip','cidr','domain','domain_suffix','domain_keyword','port')),
    value      TEXT NOT NULL,
    note       TEXT,
    is_risky   INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    CONSTRAINT chk_bypass_rule_scope CHECK (
        (scope = 'global' AND host_id IS NULL) OR
        (scope = 'host'   AND host_id IS NOT NULL)
    )
);

-- ============================================================
-- host_bypass_bindings
-- ============================================================
CREATE TABLE IF NOT EXISTS host_bypass_bindings (
    id         TEXT PRIMARY KEY,
    host_id    TEXT NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
    preset_id  TEXT REFERENCES host_bypass_presets (id) ON DELETE RESTRICT,
    rule_id    TEXT REFERENCES host_bypass_rules (id) ON DELETE CASCADE,
    enabled    INTEGER NOT NULL DEFAULT 1,
    source     TEXT NOT NULL DEFAULT 'admin' CHECK (source IN ('admin','system')),
    created_at DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    CONSTRAINT chk_bypass_binding_xor CHECK (
        (preset_id IS NOT NULL AND rule_id IS NULL) OR
        (preset_id IS NULL     AND rule_id IS NOT NULL)
    )
);

-- ============================================================
-- host_bypass_snapshots
-- ============================================================
CREATE TABLE IF NOT EXISTS host_bypass_snapshots (
    id                      TEXT PRIMARY KEY,
    host_id                 TEXT NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
    version                 INTEGER NOT NULL,
    config_hash             TEXT NOT NULL,
    whitelist_cidrs_json    TEXT NOT NULL DEFAULT '{"version":3,"rules":[]}',
    whitelist_domains_json  TEXT NOT NULL DEFAULT '{"version":3,"rules":[]}',
    applied_status          TEXT NOT NULL DEFAULT 'pending'
                            CHECK (applied_status IN ('pending','applied','failed','rolled_back')),
    source                  TEXT NOT NULL DEFAULT 'apply',
    created_by              TEXT,
    created_at              DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    UNIQUE (host_id, config_hash)
);

-- ============================================================
-- host_bypass_audit_log
-- ============================================================
CREATE TABLE IF NOT EXISTS host_bypass_audit_log (
    id          TEXT PRIMARY KEY,
    actor_id    TEXT,
    actor_ip    TEXT,
    action      TEXT NOT NULL,
    target_kind TEXT NOT NULL,
    target_id   TEXT,
    before      TEXT,
    after       TEXT,
    note        TEXT,
    created_at  DATETIME NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

-- ============================================================
-- 索引
-- ============================================================

-- tasks
CREATE INDEX IF NOT EXISTS idx_tasks_status_updated_at ON tasks (status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_tasks_stale ON tasks (status, updated_at) WHERE status IN ('pending', 'running');

-- hosts
CREATE INDEX IF NOT EXISTS idx_hosts_user_id ON hosts (user_id);
CREATE INDEX IF NOT EXISTS idx_hosts_status_running ON hosts (status) WHERE status = 'running';
CREATE UNIQUE INDEX IF NOT EXISTS idx_hosts_user_active
    ON hosts (user_id)
    WHERE status NOT IN ('deleted', 'archived');

-- host_egress_bindings
CREATE INDEX IF NOT EXISTS idx_host_egress_bindings_host_id ON host_egress_bindings (host_id);

-- events
CREATE INDEX IF NOT EXISTS idx_events_created_at ON events (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_type_created_at ON events (type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_user_id_created_at ON events (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_host_id_created_at ON events (host_id, created_at DESC);

-- claude_accounts
CREATE INDEX IF NOT EXISTS idx_claude_accounts_user_id ON claude_accounts (user_id);
CREATE INDEX IF NOT EXISTS idx_claude_accounts_host_id ON claude_accounts (host_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_claude_accounts_email ON claude_accounts (email);

-- ssh_keys
CREATE INDEX IF NOT EXISTS idx_ssh_keys_user_id ON ssh_keys (user_id);
CREATE INDEX IF NOT EXISTS idx_ssh_keys_user_purpose ON ssh_keys (user_id, purpose);

-- bypass tables
CREATE INDEX IF NOT EXISTS idx_bypass_rules_host ON host_bypass_rules (host_id) WHERE host_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bypass_bindings_host ON host_bypass_bindings (host_id);
CREATE INDEX IF NOT EXISTS idx_bypass_snapshots_host_version ON host_bypass_snapshots (host_id, version DESC);
CREATE INDEX IF NOT EXISTS idx_bypass_audit_target ON host_bypass_audit_log (target_kind, target_id);
CREATE INDEX IF NOT EXISTS idx_bypass_audit_created ON host_bypass_audit_log (created_at DESC);

-- ============================================================
-- 系统预设 seed
-- ============================================================
INSERT OR IGNORE INTO host_bypass_presets (id, slug, name, description, is_system, is_force_on, is_active, rules)
VALUES
  (hex(randomblob(16)), 'loopback', '本机回环',
   '127.0.0.0/8 与 169.254.0.0/16（链路本地），强制开启不可关闭。',
   1, 1, 1,
   '[{"rule_type":"cidr","value":"127.0.0.0/8"},{"rule_type":"cidr","value":"169.254.0.0/16"}]'),
  (hex(randomblob(16)), 'lan', '局域网',
   'RFC1918（10/8、172.16/12、192.168/16）+ CGNAT 100.64/10。',
   1, 0, 1,
   '[{"rule_type":"cidr","value":"10.0.0.0/8"},{"rule_type":"cidr","value":"172.16.0.0/12"},{"rule_type":"cidr","value":"192.168.0.0/16"},{"rule_type":"cidr","value":"100.64.0.0/10"}]');
