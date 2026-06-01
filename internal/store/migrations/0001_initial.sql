CREATE TABLE users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE TABLE hosts (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',
    template_image_ref TEXT NOT NULL,
    home_volume_name TEXT NOT NULL,
    slot_key TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    UNIQUE (user_id, slot_key)
);

CREATE TABLE egress_ips (
    id TEXT PRIMARY KEY,
    label TEXT NOT NULL,
    ip_address TEXT NOT NULL UNIQUE,
    provider TEXT NOT NULL DEFAULT 'manual',
    status TEXT NOT NULL DEFAULT 'available',
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE TABLE host_egress_bindings (
    id TEXT PRIMARY KEY,
    host_id TEXT NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
    egress_ip_id TEXT NOT NULL REFERENCES egress_ips (id) ON DELETE RESTRICT,
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    UNIQUE (host_id, egress_ip_id)
);

CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    host_id TEXT REFERENCES hosts (id) ON DELETE SET NULL,
    kind TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'canceled')),
    requested_by TEXT NOT NULL,
    error_code TEXT,
    error_message TEXT,
    last_error_summary TEXT,
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE TABLE events (
    id TEXT PRIMARY KEY,
    task_id TEXT REFERENCES tasks (id) ON DELETE SET NULL,
    host_id TEXT REFERENCES hosts (id) ON DELETE SET NULL,
    level TEXT NOT NULL DEFAULT 'info',
    type TEXT NOT NULL,
    message TEXT NOT NULL,
    metadata TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX idx_tasks_status_updated_at ON tasks (status, updated_at DESC);
CREATE INDEX idx_hosts_user_id ON hosts (user_id);
CREATE INDEX idx_host_egress_bindings_host_id ON host_egress_bindings (host_id);
