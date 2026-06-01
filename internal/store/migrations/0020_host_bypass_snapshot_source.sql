-- 0020_host_bypass_snapshot_source.sql
-- v3.5 Phase 46 Plan 02：为 host_bypass_snapshots 表加 source 列，区分 apply / rollback 写入路径。
--
-- 背景：Plan 02 Rollback 接口需要新建一行 source='rollback' 的 snapshot 而非修改 target 状态。
-- Phase 45 Plan 03 在 SUMMARY 内描述了 source 列，但 0019 migration 实际未建；此 migration 补上。
--
-- 回滚路径（运维手工执行，本 migrator 仅做 up）：
--   ALTER TABLE host_bypass_snapshots DROP COLUMN IF EXISTS source;

ALTER TABLE host_bypass_snapshots
    ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'apply';

-- Note: SQLite does not support ADD CONSTRAINT on existing tables.
-- The CHECK constraint is enforced at the application level.
