-- 0019_host_bypass_rules.sql
-- v3.5 Phase 45 Plan 03：网络白名单/绕过规则的数据基础设施
-- 对齐需求 BYPASS-DATA-01..04：五张表 + 两条系统预设 seed（loopback 强制开启 / lan 可选）
-- 命名风格沿用现有 host_egress_bindings：snake_case + host_ 前缀；UUID 主键 + TIMESTAMPTZ + JSONB
-- 回滚路径（运维手工执行，本 migrator 仅做 up）：
--   DROP TABLE IF EXISTS host_bypass_audit_log;
--   DROP TABLE IF EXISTS host_bypass_snapshots;
--   DROP TABLE IF EXISTS host_bypass_bindings;
--   DROP TABLE IF EXISTS host_bypass_rules;
--   DROP TABLE IF EXISTS host_bypass_presets;

BEGIN;

CREATE TABLE IF NOT EXISTS host_bypass_presets (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug          TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    description   TEXT,
    is_system     BOOLEAN NOT NULL DEFAULT FALSE,
    is_force_on   BOOLEAN NOT NULL DEFAULT FALSE,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    rules         JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS host_bypass_rules (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope        TEXT NOT NULL CHECK (scope IN ('global', 'host')),
    host_id      UUID REFERENCES hosts(id) ON DELETE CASCADE,
    rule_type    TEXT NOT NULL CHECK (rule_type IN ('ip','cidr','domain','domain_suffix','domain_keyword','port')),
    value        TEXT NOT NULL,
    note         TEXT,
    is_risky     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- scope='host' 必须带 host_id；scope='global' 必须不带（XOR 约束）
    CONSTRAINT chk_bypass_rule_scope CHECK (
        (scope = 'global' AND host_id IS NULL) OR
        (scope = 'host'   AND host_id IS NOT NULL)
    )
);
CREATE INDEX IF NOT EXISTS idx_bypass_rules_host ON host_bypass_rules(host_id) WHERE host_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS host_bypass_bindings (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id     UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    preset_id   UUID REFERENCES host_bypass_presets(id) ON DELETE RESTRICT,
    rule_id     UUID REFERENCES host_bypass_rules(id)   ON DELETE CASCADE,
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    source      TEXT NOT NULL DEFAULT 'admin' CHECK (source IN ('admin','system')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- preset_id / rule_id 必须 XOR（恰好一个非空）
    CONSTRAINT chk_bypass_binding_xor CHECK (
        (preset_id IS NOT NULL AND rule_id IS NULL) OR
        (preset_id IS NULL     AND rule_id IS NOT NULL)
    )
);
CREATE INDEX IF NOT EXISTS idx_bypass_bindings_host ON host_bypass_bindings(host_id);

CREATE TABLE IF NOT EXISTS host_bypass_snapshots (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id                 UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    version                 BIGINT NOT NULL,
    config_hash             TEXT NOT NULL,
    whitelist_cidrs_json    JSONB NOT NULL DEFAULT '{"version":3,"rules":[]}'::jsonb,
    whitelist_domains_json  JSONB NOT NULL DEFAULT '{"version":3,"rules":[]}'::jsonb,
    applied_status          TEXT NOT NULL DEFAULT 'pending'
                            CHECK (applied_status IN ('pending','applied','failed','rolled_back')),
    created_by              UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (host_id, config_hash)
);
CREATE INDEX IF NOT EXISTS idx_bypass_snapshots_host_version ON host_bypass_snapshots(host_id, version DESC);

CREATE TABLE IF NOT EXISTS host_bypass_audit_log (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id     UUID REFERENCES users(id) ON DELETE SET NULL,
    actor_ip     TEXT,
    action       TEXT NOT NULL,
    target_kind  TEXT NOT NULL,
    target_id    UUID,
    before       JSONB,
    after        JSONB,
    note         TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_bypass_audit_target  ON host_bypass_audit_log(target_kind, target_id);
CREATE INDEX IF NOT EXISTS idx_bypass_audit_created ON host_bypass_audit_log(created_at DESC);

-- 系统预设 seed（is_system=true 不可删；数据层与 Phase 46 应用层双层拦截）
INSERT INTO host_bypass_presets (slug, name, description, is_system, is_force_on, is_active, rules)
VALUES
  ('loopback', '本机回环',
   '127.0.0.0/8 与 169.254.0.0/16（链路本地），强制开启不可关闭。',
   TRUE, TRUE, TRUE,
   '[{"rule_type":"cidr","value":"127.0.0.0/8"},{"rule_type":"cidr","value":"169.254.0.0/16"}]'::jsonb),
  ('lan', '局域网',
   'RFC1918（10/8、172.16/12、192.168/16）+ CGNAT 100.64/10 + ULA fc00::/7。',
   TRUE, FALSE, TRUE,
   '[{"rule_type":"cidr","value":"10.0.0.0/8"},{"rule_type":"cidr","value":"172.16.0.0/12"},{"rule_type":"cidr","value":"192.168.0.0/16"},{"rule_type":"cidr","value":"100.64.0.0/10"},{"rule_type":"cidr","value":"fc00::/7"}]'::jsonb)
ON CONFLICT (slug) DO NOTHING;

COMMIT;
