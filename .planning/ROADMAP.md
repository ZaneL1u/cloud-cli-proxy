# Roadmap: Cloud CLI Proxy

## Milestones

- ✅ **v1.0 MVP** — Phases 1-6 (shipped 2026-03-28) — [Archive](milestones/v1.0-ROADMAP.md)
- ✅ **v1.1 支持代理协议出网** — Phases 7-10 (shipped 2026-03-28) — [Archive](milestones/v1.1-ROADMAP.md)
- ⏸ **v1.2 用户自助面板与 Bootstrap 重设计** — Phases 11-16 (paused 2026-04-03)
- 🚧 **v1.3 管理后台凭据与 SSH 密钥增强** — Phases 17-20 (in progress)

## Phases

<details>
<summary>✅ v1.0 MVP (Phases 1-6) — SHIPPED 2026-03-28</summary>

- [x] Phase 1: 基础控制面与主机代理 (3/3 plans) — completed 2026-03-26
- [x] Phase 2: 隧道出网强制层 (3/3 plans) — completed 2026-03-27
- [x] Phase 3: 启动入口与 SSH 接入 (3/3 plans) — completed 2026-03-27
- [x] Phase 4: 后台管理界面 (3/3 plans) — completed 2026-03-27
- [x] Phase 5: 到期、审计与清理 (3/3 plans) — completed 2026-03-27
- [x] Phase 6: 加固与 MVP 就绪 (4/4 plans) — completed 2026-03-28

</details>

<details>
<summary>✅ v1.1 支持代理协议出网 (Phases 7-10) — SHIPPED 2026-03-28</summary>

- [x] Phase 7: 数据层与类型化 (3/3 plans) — completed 2026-03-28
- [x] Phase 8: SingBoxProvider 与受管镜像 (3/3 plans) — completed 2026-03-28
- [x] Phase 9: 前端适配与代理测试 (3/3 plans) — completed 2026-03-28
- [x] Phase 10: 技术债务清理 (2/2 plans) — completed 2026-03-28

</details>

<details>
<summary>⏸ v1.2 用户自助面板与 Bootstrap 重设计 (Phases 11-16) — PAUSED 2026-04-03</summary>

- [x] Phase 11: 认证基础设施与数据迁移 (completed 2026-03-29)
- [x] Phase 12: 用户自助 API 与前端路由 (completed 2026-03-29)
- [ ] Phase 13: 账号管理与用户资源视图 (deferred)
- [ ] Phase 14: KasmVNC 用户面 (deferred)
- [ ] Phase 15: Bootstrap 重设计 (deferred)
- [ ] Phase 16: 级联禁用与到期治理 (deferred)

</details>

### 🚧 v1.3 管理后台凭据与 SSH 密钥增强

**Milestone Goal:** 完成管理后台的时区标准化、出口 IP 交互重构、密码可控创建与重置，以及 SSH 双密钥模型和实时容器同步能力。

- [ ] **Phase 17: 时区与出口 IP 交互重构** — 全量时区 UTC 标识 + 出口 IP Dialog 化
- [ ] **Phase 18: 用户密码可控创建与重置** — 创建可填/可生密码 + 详情页密码修改
- [ ] **Phase 19: SSH 双密钥模型与多条管理** — 登录授权公钥与外联身份密钥对拆分建模
- [ ] **Phase 20: SSH 密钥实时同步与收敛** — 运行中实时同步 + 离线启动补齐

## Phase Details

### Phase 17: 时区与出口 IP 交互重构
**Goal**: 让管理员在所有时区选择点看到完整时区清单与 UTC 偏移，并将出口 IP 创建/编辑体验统一为 Dialog
**Depends on**: Phase 12 (admin UI 基线)
**Requirements**: TZ-01, TZ-02, UI-01, UI-02
**Success Criteria** (what must be TRUE):
  1. 所有时区选择器都能选择完整时区列表，无缺项
  2. 时区选项统一显示 UTC 偏移格式（UTC±HH:MM）且与真实偏移一致
  3. 出口 IP 的创建和编辑都通过 Dialog 完成，右侧抽屉入口下线
  4. 迁移为 Dialog 后，原有校验、提交、错误提示行为不退化
**Plans**: TBD
**UI hint**: yes

### Phase 18: 用户密码可控创建与重置
**Goal**: 让管理员在创建用户和用户详情页都能控制登录密码与 SSH 密码（手填或生成）
**Depends on**: Phase 17
**Requirements**: CRED-01, CRED-02, CRED-03, CRED-04, CRED-05
**Success Criteria** (what must be TRUE):
  1. 创建用户时可分别填写登录密码和 SSH 密码，任一留空会自动生成
  2. 自动生成密码只在必要场景展示一次，后续不以明文回显
  3. 用户详情页可分别修改登录密码和 SSH 密码，支持手填与一键生成
  4. 提交前目标密码项必须有有效值，空值提交会被阻止并提示
  5. 修改成功后后端持久化和鉴权逻辑保持一致，不影响现有登录流程
**Plans**: TBD

### Phase 19: SSH 双密钥模型与多条管理
**Goal**: 建立登录授权公钥与外联身份密钥对的双模型，并支持每类多条记录管理
**Depends on**: Phase 18
**Requirements**: KEYA-01, KEYA-02, KEYI-01, KEYI-02
**Success Criteria** (what must be TRUE):
  1. 数据模型和 API 明确区分登录授权公钥与外联身份密钥对
  2. 每个用户可维护多条登录授权公钥，并可新增、更新、删除
  3. 每个用户可维护多组外联身份密钥对，并可新增、更新、删除
  4. 管理后台在用户详情页按两类密钥分区展示，避免概念混用
**Plans**: TBD
**UI hint**: yes

### Phase 20: SSH 密钥实时同步与收敛
**Goal**: 让 SSH 密钥变更能快速收敛到容器运行态，并保证离线容器在启动后状态一致
**Depends on**: Phase 19
**Requirements**: KEYS-01, KEYS-02
**Success Criteria** (what must be TRUE):
  1. 运行中容器在密钥新增、更新、删除后可实时同步成功
  2. 容器离线时，密钥变更会在下次启动自动补齐，不产生脏状态
  3. 同步失败可观测并可重试，保证容器内 authorized_keys/identity key 与数据库一致
**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 17 → 18 → 19 → 20

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. 基础控制面与主机代理 | v1.0 | 3/3 | Complete | 2026-03-26 |
| 2. 隧道出网强制层 | v1.0 | 3/3 | Complete | 2026-03-27 |
| 3. 启动入口与 SSH 接入 | v1.0 | 3/3 | Complete | 2026-03-27 |
| 4. 后台管理界面 | v1.0 | 3/3 | Complete | 2026-03-27 |
| 5. 到期、审计与清理 | v1.0 | 3/3 | Complete | 2026-03-27 |
| 6. 加固与 MVP 就绪 | v1.0 | 4/4 | Complete | 2026-03-28 |
| 7. 数据层与类型化 | v1.1 | 3/3 | Complete | 2026-03-28 |
| 8. SingBoxProvider 与受管镜像 | v1.1 | 3/3 | Complete | 2026-03-28 |
| 9. 前端适配与代理测试 | v1.1 | 3/3 | Complete | 2026-03-28 |
| 10. 技术债务清理 | v1.1 | 2/2 | Complete | 2026-03-28 |
| 11. 认证基础设施与数据迁移 | v1.2 | 1/3 | Complete | 2026-03-29 |
| 12. 用户自助 API 与前端路由 | v1.2 | 2/2 | Complete | 2026-03-29 |
| 13. 账号管理与用户资源视图 | v1.2 | 0/0 | Deferred | - |
| 14. KasmVNC 用户面 | v1.2 | 0/0 | Deferred | - |
| 15. Bootstrap 重设计 | v1.2 | 0/0 | Deferred | - |
| 16. 级联禁用与到期治理 | v1.2 | 0/0 | Deferred | - |
| 17. 时区与出口 IP 交互重构 | v1.3 | 0/0 | Not started | - |
| 18. 用户密码可控创建与重置 | v1.3 | 0/0 | Not started | - |
| 19. SSH 双密钥模型与多条管理 | v1.3 | 0/0 | Not started | - |
| 20. SSH 密钥实时同步与收敛 | v1.3 | 0/0 | Not started | - |

---
*Last updated: 2026-04-03 — Milestone v1.3 roadmap created*
