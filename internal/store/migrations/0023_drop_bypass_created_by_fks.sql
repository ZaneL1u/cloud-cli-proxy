-- 0023_drop_bypass_created_by_fks.sql
-- 移除 host_bypass_snapshots.created_by 和 host_bypass_audit_log.actor_id 的外键约束。
--
-- 原因：这两个是审计字段，当 JWT 中的 user_id 在 users 表中不存在时（数据库重建、
-- 用户被删后重建等场景），FK 约束会导致整个 INSERT 失败，业务请求返回 500。
-- 审计字段不应阻塞业务写入 — 用户 ID 只是记录"谁做了这个操作"，即使该用户已不存在，
-- 保留原始 ID 对审计追溯仍然有价值。
--
-- 回滚路径（运维手工执行）：
--   ALTER TABLE host_bypass_snapshots ADD CONSTRAINT host_bypass_snapshots_created_by_fkey
--     FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
--   ALTER TABLE host_bypass_audit_log ADD CONSTRAINT host_bypass_audit_log_actor_id_fkey
--     FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE SET NULL;

BEGIN;

-- 条件块确保 migration 可重放
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'host_bypass_snapshots_created_by_fkey'
    ) THEN
        ALTER TABLE host_bypass_snapshots
            DROP CONSTRAINT host_bypass_snapshots_created_by_fkey;
    END IF;

    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'host_bypass_audit_log_actor_id_fkey'
    ) THEN
        ALTER TABLE host_bypass_audit_log
            DROP CONSTRAINT host_bypass_audit_log_actor_id_fkey;
    END IF;
END $$;

COMMIT;
