-- Cloud CLI Proxy e2e fixture：postgres seed
--
-- 说明：
--   - 本文件 *不* 做表结构 migration；控制面启动时通过 internal/store/migrations
--     自动跑 0001..0020 完整迁移，e2e 直接复用生产 migration 链。
--   - 本文件仅做 *e2e 专用* 的数据级 fixture：例如必要的 admin 用户占位、
--     特殊测试场景下需要的 seed 数据。
--   - 真实生产数据不允许出现：所有密码、token、邮箱必须是 e2e 占位字面量。
--
-- 当前 Phase 45 Plan 02 尚不需要预置任何行 —— 控制面在 Scenario.Start 中
-- 通过 admin API 动态创建 fixture user / egress IP / host 三件套（Plan 02
-- 后续阶段 / Plan 04 接入）。留这个文件作为后续 phase 的扩展挂点。

-- 占位 no-op，保证 psql -f 不报错。
SELECT 1 AS e2e_fixture_loaded;
