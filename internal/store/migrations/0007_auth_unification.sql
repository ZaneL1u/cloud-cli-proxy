-- 1. users 表增加 role 列（per D-04）
ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'user';

-- 2. claude_accounts 表（per D-09, D-10）
CREATE TABLE IF NOT EXISTS claude_accounts (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    host_id TEXT REFERENCES hosts (id) ON DELETE SET NULL,
    email TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_claude_accounts_user_id ON claude_accounts (user_id);
CREATE INDEX IF NOT EXISTS idx_claude_accounts_host_id ON claude_accounts (host_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_claude_accounts_email ON claude_accounts (email);
