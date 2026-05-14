# Phase 45: 测试基础设施与 CI 骨架 - Context

**Gathered:** 2026-05-14
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous, all 4 grey areas accepted)

<domain>
## Phase Boundary

在本仓库长出可复用的端到端测试骨架（testcontainers-go + testify/suite + Scenario 抽象 + 双层 CI 触发策略 + 失败 artifact 自动归档 + waitFor 替代裸 sleep），让 v3.6 后续 phase（46–52）能基于同一套 harness 写真实跑得通的网络栈用例。

**本 phase 交付的是骨架，不是真实业务用例**：bootstrap、出口 IP、DNS、kill-switch、防泄漏等用例在后续 phase 落地。Phase 45 只需提供：
- 一个能跑通最小烟雾用例的 e2e 套件
- 一个能声明式描述"控制面 + host-agent + Postgres + N 个用户容器 + sing-box gateway"的 Scenario 抽象
- 一组 waitFor helper（含 Log/Port/HTTP/Exec 4 个常用变体）
- e2e 失败时自动归档 artifact 的 hook
- 一个新的 GitHub Actions job（hosted ubuntu-24.04 runner）按 `paths` 过滤强制守护 e2e 通过

**不在本 phase 范围**：真实业务用例（属 Phase 46+）、性能优化与基线快照、self-hosted runner 接入、Tetragon 内核观测（属 v2 范围）。

</domain>

<decisions>
## Implementation Decisions

### Area 1: 测试框架与库选型 (E2E-01)
- **容器编排库**：testcontainers-go（ROADMAP 明确指定，复用其 image build / cleanup hooks）
- **套件组织**：testify/suite（提供 SetupSuite / TearDownSuite 生命周期，便于多用例共享 fixture）
- **断言库**：testify/require（fail-fast；e2e 失败 case 后续断言无意义，避免雪崩日志）
- **go.mod 隔离**：同一 go.mod，新增 `tests/e2e/` 子目录；testcontainers-go 等依赖纳入主 go.sum，不引入 multi-module 矩阵复杂度

### Area 2: Scenario 抽象的形态 (E2E-02)
- **Scenario API 形态**：Go builder API，例如 `scenario.New(t).WithControlPlane().WithHosts(2).WithUsers(3).Start()`；类型化、IDE 跳转友好、断言可复用
- **fixture 是否复用 dev compose**：新建专用 fixture（不复用 `docker-compose.dev.yml`）；dev compose 含开发期热重载 / 挂载，会污染 e2e 洁净副本
- **多容器编排归属**：testcontainers-go 直接管理控制面 / Postgres / host-agent；sing-box gateway + worker 沿用现有 `ContainerProxyProvider` / host-agent 真实链路启动，保证"测的就是生产路径"
- **基线快照**：v3.6 不做（image cache + Postgres dump 属性能优化，留到后续 milestone 评估）

### Area 3: CI 双层架构与触发策略 (E2E-03 / E2E-04)
- **特权网络 e2e 跑在哪**：hosted `ubuntu-24.04` runner（与 v3.5 `uat-bypass.yml` 同款，内部 `sudo` 起 docker / nft；免维护、零运维成本）
  - **注意 ROADMAP 调整**：v3.6 ROADMAP 草稿中 E2E-03 描述"self-hosted Linux runner 跑特权网络栈 e2e"。Phase 45 实际落地为 hosted ubuntu-24.04，不引入 self-hosted runner 运维负担。Phase 45 SUMMARY 中需明确记录这一调整原因
- **e2e 触发策略**：PR `paths` 过滤 + 强制守护；命中 `tests/e2e/**` / `internal/network/**` / `cmd/host-agent/**` / `internal/runtime/**` 路径就强制跑且 block PR merge
- **artifact 上限**：单 job 100MB 上限 + 30 天保留（GitHub 默认配额内，足够装抓包 / 日志 / dump）
- **失败时归档策略**：失败时无条件归档（`actions/upload-artifact@v4` + `if: failure()`），并在 PR 评论里贴下载链接，开发者无需进 Actions tab 找

### Area 4: waitFor / 同步原语风格 (E2E-05)
- **waitFor helper 实现**：自己写 `tests/e2e/harness/waitfor.go`，签名 `WaitFor(ctx, name string, predicate func() error, opts ...WaitOpt)`；错误信息可控、可与 Scenario 集成 dump artifact
- **默认参数**：`timeout=30s` / `pollInterval=500ms`，可被 opts 覆盖（`WithTimeout` / `WithPollInterval`）
- **专用变体**：同时提供 `WaitForLog` / `WaitForPort` / `WaitForHTTP` / `WaitForExec` 4 个常用变体，避免每个用例自写 boilerplate
- **超时失败行为**：超时立即触发 artifact dump（容器日志 + nft ruleset + netns + route），再返回错误；确保排障证据 1:1 对应该次失败

### Claude's Discretion
以下细节在实现层由 Claude 自行决定，无需用户拍板：
- testify/suite 的具体 suite 文件命名、目录结构（建议：`tests/e2e/<feature>_test.go` + `tests/e2e/harness/`）
- testcontainers-go 的 image build 是否复用现有 `Dockerfile.managed-user` / `Dockerfile.host-agent`（建议复用，避免镜像漂移）
- waitFor 的内部错误聚合策略（每次 predicate 失败的错误是否累计 vs 仅保留最后一次）
- artifact 目录结构具体子目录命名（与 Phase 52 OBS-02 对齐，本 phase 先建占位）
- PR 评论的具体格式与 GitHub App 选型（推荐 `actions/github-script` 直接发评论，避免引入第三方 action）
- 禁止裸 sleep 的守护方式（grep 守护脚本 + golangci-lint 自定义规则二选一，建议 grep 脚本起步）

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`internal/runtime/ContainerProxyProvider`**（v3.5 已拆 `PrepareGateway` + `PrepareHost`）：e2e 中的 sing-box gateway + worker 启动直接复用真实 provider，确保 e2e 跑的是生产路径
- **`internal/agentapi`**：host-agent 与控制面之间的 SDK / 类型定义，e2e 可直接 import 复用
- **`scripts/uat-bypass-fixture-up.sh` / `uat-bypass-fixture-down.sh`**（v3.5）：fixture 起停脚本范式，Phase 45 的 Go 版本 Scenario 抽象可参考其拓扑（postgres + host-agent + sing-box host）
- **现有 186 个 `_test.go`**：标准 Go testing；e2e suite 通过 `tests/e2e/...` 与既有 unit / integration 测试隔离，避免污染

### Established Patterns
- **Go 测试目录约定**：`internal/<pkg>/<file>_test.go` 与被测文件同目录；e2e 例外，独立 `tests/e2e/`
- **CI 工作流分层**：v3.5 `uat-bypass.yml` 已建立"lint job 永远跑 + uat job preflight 自适应"模式，Phase 45 的 e2e workflow 沿用同一思路
- **结构化日志**：`log/slog` key-value，e2e harness 输出建议同源风格，便于 artifact grep
- **错误处理**：显式 `if err != nil` + `fmt.Errorf("...: %w", err)` 包装上下文；waitFor 的错误返回沿用此风格
- **中文沟通**：`.planning/codebase/CONVENTIONS.md` 与 `CLAUDE.md` 明确所有面向用户的输出（含 PR 评论、artifact README）使用中文；e2e 失败评论的提示文案默认中文

### Integration Points
- **新增目录**：`tests/e2e/`（suite 入口）、`tests/e2e/harness/`（Scenario builder + waitFor + artifact 收集）、`tests/e2e/fixtures/`（专用 docker / SQL 种子）
- **新增 CI workflow**：`.github/workflows/e2e.yml`（hosted ubuntu-24.04，paths 过滤 + 强制守护 + 失败 artifact 上传 + PR comment）
- **go.mod 新增依赖**：`github.com/testcontainers/testcontainers-go`、`github.com/stretchr/testify`（首次引入 testify）；通过 `go mod tidy` 落地，确保不引入间接 v0 依赖
- **lint 守护**：新增脚本 `scripts/lint-no-bare-sleep.sh`（grep `time.Sleep` in `tests/e2e/`），加入 `ci.yml` 必跑步骤
- **隐私守护**：harness 输出与 fixture SQL 严禁出现真实邮箱 / 路径，沿用 CLAUDE.md 与 CONVENTIONS.md 规则

</code_context>

<specifics>
## Specific Ideas

- **Scenario builder 命名风格参考**：`scenario.New(t).WithControlPlane().WithSingBoxGateway("singapore").WithHost(...).WithUser(...).Start(ctx)`；返回 `*Scenario`，含 `ControlPlane()` / `Host(name)` / `User(name)` 等访问器
- **WaitFor 错误格式参考**：`WaitFor name=host-agent.healthy: timed out after 30s; last err: dial tcp 127.0.0.1:8081: connection refused`
- **artifact 目录结构占位**：Phase 45 先建立 `logs/` / `network/` / `docker/` / `postgres/` / `system/` 五个空子目录的占位脚本，Phase 52 (OBS-01..03) 再补完整收集逻辑
- **e2e workflow 名字**：建议 `e2e.yml`，job 名 `e2e (ubuntu-24.04)`；fixture preflight 风格参考 `uat-bypass.yml`，自适应跳过未就绪 fixture（Phase 45 自身可能只跑最小烟雾）

</specifics>

<deferred>
## Deferred Ideas

- **Tetragon TracingPolicy 接入**：作为内核级 oracle 的 e2e 验证，属 v2 范围（REQUIREMENTS 已明确 deferred）
- **基线快照 / image cache 加速**：本 phase 不做，纳入后续性能优化候选
- **self-hosted Linux runner 接入**：本 phase 不做，留待 e2e 用例规模扩到 hosted runner 性能瓶颈时再评估
- **跨 phase 共享 fixture 缓存**：随 Phase 47 / 49 用例规模而定，本 phase 仅提供单次起停 fixture 的能力
- **lint 守护从 grep 升级为 golangci-lint 自定义规则**：grep 起步，后续视回归压力升级
- **PR 评论的多语言切换**：当前默认中文（与项目沟通约定一致），暂不支持英文切换
- **e2e workflow 与现有 uat-bypass.yml 合并**：保持独立 job，避免 fixture preflight 互相耦合；后续 phase 视复用收益再评估

</deferred>
