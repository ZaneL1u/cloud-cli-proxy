-- User expiry field
ALTER TABLE users ADD COLUMN expires_at TEXT;

-- Extend events table with user_id column
ALTER TABLE events ADD COLUMN user_id TEXT REFERENCES users (id) ON DELETE SET NULL;

-- Event query indexes
CREATE INDEX idx_events_created_at ON events (created_at DESC);
CREATE INDEX idx_events_type_created_at ON events (type, created_at DESC);
CREATE INDEX idx_events_user_id_created_at ON events (user_id, created_at DESC);
CREATE INDEX idx_events_host_id_created_at ON events (host_id, created_at DESC);

-- Reconciliation scan indexes
CREATE INDEX idx_hosts_status_running ON hosts (status) WHERE status = 'running';
CREATE INDEX idx_tasks_stale ON tasks (status, updated_at) WHERE status IN ('pending', 'running');
