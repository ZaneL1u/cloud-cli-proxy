-- Host resource limits: memory, CPU, disk quota for container creation
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS memory_limit_mb INTEGER NOT NULL DEFAULT 4096;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS cpu_limit REAL NOT NULL DEFAULT 2.0;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS disk_limit_gb INTEGER NOT NULL DEFAULT 20;
