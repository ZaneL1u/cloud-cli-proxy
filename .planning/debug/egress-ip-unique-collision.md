---
status: investigating
trigger: "创建第二条未检测出口线路时，ip_address=0.0.0.0 触发 SQLite 唯一约束；需要代码修复并发布"
created: 2026-07-13T00:00:00Z
updated: 2026-07-13T00:00:00Z
---

## Current Focus

hypothesis: egress_ips.ip_address 被错误用作唯一身份，但前端对所有未检测代理都写入 0.0.0.0，且多个代理也可能共享真实出口 IP，因此第二次创建或自动回填必然触发唯一冲突
test: 先用旧 schema 复现重复 IP 插入失败，再应用移除唯一约束的迁移并验证绑定与外键完整
expecting: 迁移后允许不同出口资源拥有相同或空的 ip_address，同时 host_egress_bindings 数据和约束保持不变
next_action: 编写迁移、API 与前端失败测试
reasoning_checkpoint: 已排除临时分配 192.0.2.x 和把 label 改为唯一字段；前者污染数据语义，后者可能让历史重复标签阻断升级
tdd_checkpoint: pending-red

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

## Resolution

root_cause: ""
fix: ""
verification: ""
files_changed: []
