---
status: verifying
trigger: "创建第二条未检测出口线路时，ip_address=0.0.0.0 触发 SQLite 唯一约束；需要代码修复并发布"
created: 2026-07-13T00:00:00Z
updated: 2026-07-13T09:12:30Z
---

## Current Focus

hypothesis: 已确认；旧 schema 在第二条重复 ip_address 插入时稳定报 UNIQUE constraint failed，移除该约束后同一测试通过
test: 迁移、API 和前端回归测试均完成红灯到绿灯；Go race、e2e、前端单测和生产构建已通过
expecting: 提交并合入 main 后，v4.2.19 Release workflow、GitHub Release 和容器镜像全部成功
next_action: 提交、推送、合入 main 并创建 v4.2.19 标签
reasoning_checkpoint: 已排除临时分配 192.0.2.x 和把 label 改为唯一字段；前者污染数据语义，后者可能让历史重复标签阻断升级
tdd_checkpoint: red-green-complete

## Symptoms

expected: 可以创建多条尚未探测真实 IP 的代理线路，也可以保存共享同一出口 IP 的不同线路
actual: 第一条线路占用 0.0.0.0 后，第二条创建返回唯一约束冲突；真实 IP 自动纠正也可能因重复地址失败
errors: "ip address already exists" / SQLite UNIQUE constraint failed
reproduction: 连续创建两条不填写出口 IP 的代理线路
started: v4.0 引入统一 0.0.0.0 占位后

## Eliminated

- hypothesis: 为每条线路分配 192.0.2.x 占位地址可以根治
  reason: 只能规避创建冲突，仍会在共享真实出口 IP 时失败，并把保留网段伪装成业务数据
- hypothesis: 把唯一约束迁移到 label 可以安全替代
  reason: label 不是资源身份，历史重复标签会导致迁移失败；资源已有 id 主键

## Evidence

- timestamp: 2026-07-13T00:00:00Z
  checked: web/admin/src/components/egress-ips/egress-ip-drawer.tsx
  found: 未填写 ip_address 时无条件写入 0.0.0.0
  implication: 所有尚未检测的线路竞争同一个数据库唯一值
- timestamp: 2026-07-13T00:00:00Z
  checked: internal/store/migrations/0001_initial.sql
  found: egress_ips.ip_address 定义为 TEXT NOT NULL UNIQUE，而 id 已经是 PRIMARY KEY
  implication: ip_address 承担了不应有的资源身份职责
- timestamp: 2026-07-13T00:00:00Z
  checked: internal/store/repository/queries.go 和出口探测流程
  found: 探测会写入 detected_ip_address，运行时自动纠正还会更新 ip_address；不同线路可能得到同一实际出口地址
  implication: 唯一约束不仅影响占位值，也会影响合法的共享出口场景
- timestamp: 2026-07-13T00:00:00Z
  checked: GitHub PR #8
  found: PR 同样识别到 ip_address 唯一约束问题，但同时引入 label UNIQUE 并夹带安全、VNC、SSH 等无关改动
  implication: 本次应采用不新增业务唯一键的聚焦补丁发布
- timestamp: 2026-07-13T09:07:00Z
  checked: 新增迁移、API 和前端回归测试的红灯结果
  found: 旧 schema 拒绝第二条 0.0.0.0；Create 空 IP 返回 400；Update 非法 IP 返回 200；前端规范化模块尚不存在
  implication: 三组测试分别命中数据库、API 和前端根因，不是无关环境失败
- timestamp: 2026-07-13T09:08:00Z
  checked: 最小实现后的定向测试
  found: 迁移测试、API 测试和前端测试全部通过；迁移测试同时验证重复标签、重复真实 IP、重复空 IP、绑定保留、RESTRICT 和 foreign_key_check
  implication: 修复覆盖了数据模型冲突且未削弱绑定完整性
- timestamp: 2026-07-13T09:10:12Z
  checked: 扩大验证和 origin/main 类型检查基线
  found: go test ./...、34 个前端单测和 Vite 生产构建通过；typecheck 的剩余错误均可在 origin/main 复现，本分支未新增类型错误
  implication: 当前改动没有观察到 Go、前端运行构建或类型层面的新增回归
- timestamp: 2026-07-13T09:12:30Z
  checked: CI 等价验证与提交前代码审查
  found: 非 e2e Go 包在 race 和随机顺序下通过，e2e 包通过，前端 34 个单测与生产构建再次通过；审查确认空 ExpectedIP 会走现有 UpdateExpectedIP 回调自动回填
  implication: 修复满足当前 CI 质量门，且未破坏首次出口探测与自动纠正流程

## Resolution

root_cause: egress_ips.id 已是主键，但 ip_address 仍被定义为 UNIQUE；前端又对所有未检测线路写入相同的 0.0.0.0，导致第二次创建和共享真实出口 IP 的自动回填发生合法数据冲突。
fix: 新增 SQLite 表重建迁移移除 ip_address 唯一约束并保留 NOT NULL、绑定数据和外键；API 允许空 IP 但拒绝非法非空 IP；前端空值不再改写为 0.0.0.0。
verification: 定向红绿测试、go test ./...、Go race 随机顺序测试、e2e、前端 34 个单测和生产构建已通过；typecheck 仅剩 origin/main 已存在的错误。等待发布工作流。
files_changed:
  - internal/store/migrations/0003_drop_egress_ip_address_unique.sql
  - internal/store/repository/migration_0003_test.go
  - internal/controlplane/http/admin_egress_ips.go
  - internal/controlplane/http/admin_egress_ips_test.go
  - web/admin/src/components/egress-ips/egress-ip-drawer.tsx
  - web/admin/src/lib/egress-ip-address.ts
  - web/admin/src/lib/__tests__/egress-ip-address.test.ts
