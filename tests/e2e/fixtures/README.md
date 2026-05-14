# Cloud CLI Proxy e2e fixture

本目录为 Phase 45 Plan 02 引入的 *e2e 专用* fixture，与 `docker-compose.dev.yml` 完全独立，专门服务 `tests/e2e/...` 套件。

## 设计原则

- **不复用 dev compose**：开发期热重载 / 源码挂载会污染 e2e 洁净副本（CONTEXT.md §Area 2 锁定）。
- **起停由 Scenario 全权负责**：用例代码通过 `scenario.New(t)...Start(ctx)` 起，`Scenario.Stop` 在 `TearDownTest` 自动调用，**没有需要人工运行的脚本**。
- **镜像复用生产构建**：`postgres:18`、`cloud-cli-proxy-sing-gateway:local`、`Dockerfile.managed-user` 全部沿用主仓库已经构建好的镜像，避免镜像漂移。

## 拓扑

`Scenario.Start` 内部按顺序拉起以下组件（Plan 02 当前阶段仅完成 Step 1；Step 2..7 留 TODO，由 Plan 02 后续阶段或独立 plan 推进真实代码）：

1. **Step 1**：Postgres testcontainer（`postgres:18`，端口 5432）— 控制面后端存储 ✅ 已实现
2. **Step 2**：Control plane 子进程（`go run ./cmd/control-plane`，绑定动态端口）— admin / user / host API ⏸ TODO
3. **Step 3**：通过 admin API 颁发 admin token + 创建 fixture user / egress IP / host 三件套 ⏸ TODO
4. **Step 4..6**：每个声明的 sing-box gateway 走真实 `internal/network.ContainerProxyProvider.PrepareGateway` ⏸ TODO
5. **Step 7**：每个声明的 host 走真实 `provider.PrepareHost` 并等 verify 通过 ⏸ TODO

注：本 Phase 45 Plan 02 暂不在 e2e 中启动独立的 `cmd/host-agent` 进程；Scenario 直接复用 in-process `ContainerProxyProvider`，保证 e2e 测试的就是真实 docker / nft 路径，又避免 host-agent socket 多进程协同复杂度。Phase 46+ 真实业务用例（bootstrap / SSH banner 等）落地时若需要 host-agent 进程，由后续 plan 扩展 Scenario。

## 数据级 fixture

- `postgres-seed.sql`：占位文件，本 plan 阶段暂不预置行；后续 phase 如需 e2e 专属 seed（例如某条特殊 expired user），按"幂等 INSERT ON CONFLICT DO NOTHING"模式追加。

## 严禁

- 绝对路径（`/Users/...`、`/home/...`、`C:\...`）
- 真实邮箱 / token / 密码：用占位符如 `e2e-postgres-pw`、`test@example.com`
- 引用 `docker-compose.dev.yml` 或 `deploy/dev/` 下的开发挂载
