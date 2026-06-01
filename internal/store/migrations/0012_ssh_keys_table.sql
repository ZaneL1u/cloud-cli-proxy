CREATE TABLE IF NOT EXISTS ssh_keys (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose     TEXT NOT NULL CHECK (purpose IN ('inbound', 'outbound')),
    label       TEXT NOT NULL DEFAULT '',
    public_key  TEXT NOT NULL,
    private_key TEXT NOT NULL DEFAULT '',
    key_type    TEXT NOT NULL DEFAULT 'ed25519',
    fingerprint TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_ssh_keys_user_id ON ssh_keys(user_id);
CREATE INDEX IF NOT EXISTS idx_ssh_keys_user_purpose ON ssh_keys(user_id, purpose);

-- Migrate existing keys from users table
INSERT INTO ssh_keys (id, user_id, purpose, label, public_key, private_key, key_type)
SELECT hex(randomblob(16)), id, 'outbound', 'default', ssh_public_key, COALESCE(ssh_private_key, ''), COALESCE(ssh_key_type, 'ed25519')
FROM users
WHERE ssh_public_key IS NOT NULL AND ssh_public_key != '';
