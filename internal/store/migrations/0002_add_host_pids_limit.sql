-- 0002_add_host_pids_limit.sql
-- 为主机资源限制增加 Docker pids 上限，默认 1024。

ALTER TABLE hosts ADD COLUMN pids_limit INTEGER NOT NULL DEFAULT 1024;

UPDATE hosts SET pids_limit = 1024 WHERE pids_limit IS NULL;
