-- 0018: 用户中心化凭据 — 把 entry_password 提升到 users，强约束一用户一活跃 host。
--
-- 背景：v1 阶段 hosts.entry_password 与 users.entry_password 双写并存，
-- 实际 SSH 登录密码本就只跟用户绑定。Phase A1 起把所有权完全归到 users，
-- 同时硬约束「一个用户最多绑一台未删/未归档的 host」，简化所有调用路径。
--
-- 用户已确认无「一用户多 host」存量数据；存量用户若 entry_password 为空、
-- host 侧非空，则做反向回填，确保此次迁移后 SSH 仍可以原密码登录。

-- 反向回填：原 host.entry_password → user.entry_password（仅当 user 侧为空且 host 侧非空）
UPDATE users
SET entry_password = (
    SELECT h.entry_password FROM hosts h
    WHERE h.user_id = users.id
      AND COALESCE(users.entry_password, '') = ''
      AND COALESCE(h.entry_password, '') <> ''
)
WHERE EXISTS (
    SELECT 1 FROM hosts h
    WHERE h.user_id = users.id
      AND COALESCE(users.entry_password, '') = ''
      AND COALESCE(h.entry_password, '') <> ''
);

-- 删除 hosts.entry_password 列（SQLite 3.35+ 支持 DROP COLUMN）
ALTER TABLE hosts DROP COLUMN entry_password;

-- Note: SQLite partial indexes with WHERE NOT IN syntax may need care.
-- This unique index ensures one active host per user.
CREATE UNIQUE INDEX IF NOT EXISTS idx_hosts_user_active
  ON hosts (user_id)
  WHERE status NOT IN ('deleted', 'archived');
