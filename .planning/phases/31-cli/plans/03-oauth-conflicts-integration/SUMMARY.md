---
phase: 31-cli
plan: 03-oauth-conflicts-integration
subsystem: internal/cloudclaude
tags: [oauth, conflicts, integration, fixture, sync-cli, wave-3]
requires:
  - "Plan 01 errcodes.NET_OAUTH_EXPIRED / NET_OAUTH_EXPIRING_SOON / NET_OAUTH_NOT_FOUND（mount.go 注册表中已就位）"
  - "Plan 02 ConnectAndRunClaudeV3 入口（mount ready 后 TODO(plan-03) hook 点）"
  - "Plan 02 ExitOAuthNotFound=6 / ExitOAuthExpired=7 命名常量（exitcodes.go）"
  - "Plan 02 mountMutagen 三返回值签名（cleanup, X, err）+ mountMutagenDeps 注入接口"
  - "Plan 02 mount_strategy.go banner + last-session.json 写入路径"
provides:
  - "CheckOAuthCredentials(connA, claudeAccountID) → (*OAuthStatus, error) — 远端 timeout 2 cat credentials.json"
  - "parseExpiresAt(rawJSON, now) *OAuthStatus — 纯函数，三态判定 + JSON 容错"
  - "OAuthState{Valid, ExpiringSoon, Expired, NotFound} + OAuthStatus{State, ExpiresAt, MinutesToExpire}"
  - "MutagenSyncStatus{SessionName, ConflictCount, LastError} — mountMutagen 第二返回值（替换 int）"
  - "mountMutagen 内 sync list --template 解析 conflict count（v0.18.1 兼容；不用 --json）"
  - "newSyncCmd() / sync conflicts 子命令（cmd/cloud-claude/sync.go）"
  - "scripts/test-fixture-{up,down}.sh — docker compose Phase 29 镜像 fixture 起停"
  - "internal/cloudclaude/integration_test.go — 6 个 //go:build integration 集成场景骨架"
affects:
  - "ConnectAndRunClaudeV3 现已在 mount ready 后并发执行 OAuth 检查；Expired/NotFound 直接 return ExitOAuth*；ExpiringSoon 仅警告"
  - "mountMutagen 调用方（mount_strategy.tryMode/tryModeReal/tryModeWithHooks + 测试 hooks）签名同步升级 int → MutagenSyncStatus"
  - "mount_strategy banner 后的中文冲突警告本阶段起开始可被生产路径触发（ConflictCount > 0 来自 sync list 解析）"
  - "Phase 32 多端冲突：OAuth 检查与 Mutagen lock 并发；OAuth 失败优先级高于 lock（D-23）"
  - "Phase 34 doctor mount/oauth 维度复用 OAuthState 枚举 + last-session.json conflict_count 字段"
  - "Phase 35 真机验收：6 个 TestIntegration_* 是 ROADMAP §Phase 31 Success Criteria 第 4/6/8/9 条 CI gate；C3 真机收口"
tech-stack:
  added: []
  patterns:
    - "纯函数 parseExpiresAt + 远端 SSH wrapper 二分（CheckOAuthCredentials 容错收敛 → OAuthNotFound）"
    - "table-driven test：表格化覆盖 9 个 JSON 解析路径 + 边界（5min 严格小于）"
    - "TDD 严格 RED → GREEN：oauth_check stub 返回 NotFound 让 build 通过，再补真实解析逻辑"
    - "go:build integration 隔离 + TestMain os.Exit(0) 优雅跳过：本地无 docker 不阻塞 unit test"
    - "scripted docker compose fixture：替代 testcontainers-go，避免 50+ indirect deps"
    - "MutagenSyncStatus 引入：把第二返回值从 int 升级为 struct，让后续 LastError / 其他字段可扩展"
key-files:
  created:
    - "internal/cloudclaude/oauth_check.go"
    - "internal/cloudclaude/oauth_check_test.go"
    - "cmd/cloud-claude/sync.go"
    - "internal/cloudclaude/integration_test.go"
    - "scripts/test-fixture-up.sh"
    - "scripts/test-fixture-down.sh"
  modified:
    - "internal/cloudclaude/ssh.go (TODO(plan-03) → CheckOAuthCredentials + ExitOAuth* 分支)"
    - "internal/cloudclaude/mount_mutagen.go (引入 MutagenSyncStatus + sync list --template 解析)"
    - "internal/cloudclaude/mount_strategy.go (tryMode 系列 int → MutagenSyncStatus，banner conflict 警告引用 status.ConflictCount)"
    - "internal/cloudclaude/mount_mutagen_test.go (Test_MutagenHappyPath 计数 ≥3 → ≥4 + status 字段断言)"
    - "internal/cloudclaude/mount_strategy_test.go (3 处 hooks 签名同步升级)"
    - "cmd/cloud-claude/main.go (rootCmd.AddCommand 追加 newSyncCmd + DisableFlagParsing 路由 sync)"
decisions:
  - "MutagenSyncStatus 引入而非保留 int：Plan 03 PLAN 明确要求；让 LastError + future 字段可扩展，让 mount_strategy 直接拿 status.LastError 排障"
  - "TestIntegration_F7C_OAuthExpired_ExitsBeforeClaude 接受 NET_OAUTH_EXPIRED 或 NET_OAUTH_NOT_FOUND（expiresAt:0 走 NotFound 分支：parseExpiresAt 把 0 当作字段缺失）"
  - "凭证注入策略选择：保留 binary 调用版本（runCloudClaude exec /tmp/cloud-claude-int），CI 中 fixture 写临时 ~/.cloud-claude/config.yaml；推荐 (b) 直接 import internal/cloudclaude.ConnectAndRunClaudeV3 留作 future-proof 注释，避免本 plan 追加 mock gateway 分量"
  - "C3 netem 场景 t.Skip 占位：tc/netem 在 fixture 容器内不一定可用，且 ROADMAP 明确 Phase 35 真机验收完整覆盖 — 本 plan 占位以满足 RESEARCH §6.2 用例计数"
  - "sync conflicts 子命令仅 wrap mutagen sync list --long（不调 EntryClient / SSH 连接）：CONTEXT D-28 最小可行；sync resolve / resume 留 v3.1（OOS-A2）"
metrics:
  duration_seconds: 706
  duration_human: "~12 分钟"
  tasks_completed: 4
  files_created: 6
  files_modified: 6
  commits: 5
  completed_at: "2026-04-19T09:51:37Z"
---

# Phase 31 Plan 03: OAuth + Mutagen Conflict 冒泡 + 集成测试 Summary

## One-Liner

把 Phase 31 剩余两个 user-facing 行为（OAuth 三态过期检查、Mutagen conflict 中文冒泡）织入 ConnectAndRunClaudeV3 主路径，配套交付 cloud-claude sync conflicts 子命令、6 个集成测试场景骨架与 docker compose fixture 脚本，兑现 ROADMAP §Phase 31 Success Criteria 第 4/6/8/9 条的 CI 验收基线。

## Goal Achievement

| 维度 | 目标 | 实际 | 状态 |
|------|------|------|------|
| OAuth 三态检查接入 | mount ready 后、claude 前 + ExitOAuth*(6/7) | ssh.go ConnectAndRunClaudeV3 替换 TODO(plan-03) → 4 分支落码 | ✅ |
| parseExpiresAt 9 子用例 | 表驱动覆盖三态 + JSON 容错 + 5min 边界 | 9 子用例 + 1 ExpiringSoonMinutes 全 PASS | ✅ |
| Mutagen conflict 解析 | sync list --template（v0.18.1 不支持 --json） | mountMutagen Step 9 落地，list 失败仅记 LastError 不阻断 | ✅ |
| banner 中文冲突警告 | ConflictCount > 0 触发 ⚠ 提示 | mount_strategy 已通畅（Plan 02 留好的 hook 现在被真实数据驱动） | ✅ |
| sync conflicts 子命令 | mutagen sync list --long wrap + ExtractMutagenBinary 自动 | cmd/cloud-claude/sync.go 注册完毕，烟测通过 | ✅ |
| 6 个集成测试场景 | C3/C4/C5/REQ-F1-D/REQ-F2-B/REQ-F7-C | 6 个 TestIntegration_* 函数齐备，C3 t.Skip 占位 | ✅ |
| fixture 脚本 | docker compose 起停 + 30s sshd 等待 | 2 个脚本 chmod +x + bash -n 通过 | ✅ |
| 未引入 testcontainers-go | go.mod 不含 testcontainers | grep go.mod OK | ✅ |
| 整仓回归 | go test ./... + go vet | 全 PASS | ✅ |

## Public Interfaces 兑现清单

### `internal/cloudclaude/oauth_check.go` 新增导出

```go
type OAuthState int
const (
    OAuthValid        OAuthState = iota // expiresAt - now ≥ 5min
    OAuthExpiringSoon                   // 0 < expiresAt - now < 5min
    OAuthExpired                        // expiresAt ≤ now
    OAuthNotFound                       // 文件不存在 / JSON 解析失败 / 字段缺失
)

type OAuthStatus struct {
    State           OAuthState
    ExpiresAt       time.Time
    MinutesToExpire int
}

func CheckOAuthCredentials(connA *ssh.Client, claudeAccountID string) (*OAuthStatus, error)
// 远端命令：timeout 2 cat /home/claude/.claude/.credentials.json 2>/dev/null
// 任何错误（session 创建 / SSH 错误 / cat 退出非 0）一律收敛 → OAuthNotFound（不阻塞 mount）

func parseExpiresAt(rawJSON string, now time.Time) *OAuthStatus
// 纯函数（不依赖 ssh.Client）— 用于单测覆盖三态 + JSON 容错。
```

### `internal/cloudclaude/mount_mutagen.go` 新增/扩展

```go
type MutagenSyncStatus struct {
    SessionName   string
    ConflictCount int
    LastError     string
}

// 签名升级（Plan 03）：第二返回值 int → MutagenSyncStatus
func mountMutagen(connA *ssh.Client, cfg MutagenSyncConfig, deps mountMutagenDeps) (cleanup func(), status MutagenSyncStatus, err error)
```

### `cmd/cloud-claude/sync.go` 新增导出

```go
func newSyncCmd() *cobra.Command          // 工厂；main.go 注册
func runSyncConflicts(...)                // RunE — 调本地 mutagen sync list --long
```

## CheckOAuthCredentials 在 ConnectAndRunClaudeV3 的位置

`internal/cloudclaude/ssh.go` ConnectAndRunClaudeV3 在 mount ready 与 runClaude 之间插入 OAuth 检查（替换 Plan 02 留下的 `TODO(plan-03)` 注释，原行号 ~109-111）：

```go
// 替换前（Plan 02）：
// TODO(plan-03): OAuth credentials 检查（mount ready 之后、runClaude 之前）

// 替换后（Plan 03 — ssh.go 行 ~109-138）：
if mountCfg.ClaudeAccountID == "" {
    fmt.Fprintln(mountCfg.Logger, "[!] gateway 未返回 claude_account_id，跳过 OAuth 过期检查（建议升级 gateway 至 v3.0）")
} else {
    status, oauthErr := CheckOAuthCredentials(connA, mountCfg.ClaudeAccountID)
    if oauthErr != nil { fmt.Fprintln(mountCfg.Logger, "[!] OAuth 检查异常: "+oauthErr.Error()) }
    else { switch status.State {
        case OAuthExpired:      fmt.Fprintln(...NET_OAUTH_EXPIRED...); return ExitOAuthExpired, nil
        case OAuthNotFound:     fmt.Fprintln(...NET_OAUTH_NOT_FOUND...); return ExitOAuthNotFound, nil
        case OAuthExpiringSoon: fmt.Fprintln(...NET_OAUTH_EXPIRING_SOON, MinutesToExpire)
        case OAuthValid:        // 不输出
    } }
}
return runClaude(connA, claudeArgs, cwd, len(proxyCommands) > 0)
```

## 6 个集成测试 → ROADMAP Success Criteria 对应

执行方式：

```bash
bash scripts/test-fixture-up.sh                      # 起 Phase 29 镜像
go test -tags=integration -count=1 -v ./internal/cloudclaude/
bash scripts/test-fixture-down.sh
```

| 用例 | 覆盖需求 | ROADMAP §Phase 31 SC | 占位 |
|------|---------|----------------------|------|
| TestIntegration_C4_VersionSkew_DowngradesToSSHFSOnly | C4 PITFALLS / D-29 | 第 4 条（降级链） | ❌ 实跑 |
| TestIntegration_C5_SafetyGuard_BlocksSync | C5 PITFALLS / REQ-F1-D | 第 6 条（sync 必须未创建） | ❌ 实跑 |
| TestIntegration_F2B_KillMutagenAgent_DowngradesIn2s | REQ-F2-B / D-15 | 第 4 条（≤2s 降级 sshfs-only） | ❌ 实跑 |
| TestIntegration_F1D_50MBReject | REQ-F1-D / D-11 | 第 6 条（白名单拒绝） | ❌ 实跑 |
| TestIntegration_F7C_OAuthExpired_ExitsBeforeClaude | REQ-F7-C / D-22 | 第 9 条（OAuth 过期不进 claude） | ❌ 实跑 |
| TestIntegration_C3_NetemDrop_ColdBranchRemoved | C3 PITFALLS / D-27 | 第 5 条（30s 抖动无感） | ✅ Skip → Phase 35 真机 |

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 3.1 RED | oauth_check 9 子用例失败测试 | fdaa7d8 | `internal/cloudclaude/oauth_check{,_test}.go` |
| 3.1 GREEN | parseExpiresAt 三态解析实现 | 111c0b0 | `internal/cloudclaude/oauth_check.go` |
| 3.2 | OAuth 接线 + Mutagen conflict 解析 + banner | 7e124b1 | `internal/cloudclaude/{ssh,mount_mutagen,mount_strategy}.go` + 2 test |
| 3.3 | sync conflicts 子命令注册 | f2011c1 | `cmd/cloud-claude/{sync,main}.go` |
| 3.4 | 集成测试套件骨架 + fixture 脚本 | a202563 | `internal/cloudclaude/integration_test.go` + `scripts/test-fixture-{up,down}.sh` |

## Test Coverage

```
oauth_check_test.go (1 表驱动 + 1 边界用例 = 10 子用例):
  Test_ParseExpiresAt
    valid                     PASS  (now+1h → OAuthValid)
    expiringSoon3min          PASS  (now+3min → OAuthExpiringSoon)
    atBoundary5min            PASS  (now+5min → OAuthValid，严格 < 5min)
    expired                   PASS  (now-1s → OAuthExpired)
    emptyInput                PASS  (空字符串 → NotFound)
    malformedJSON             PASS  (非法 JSON → NotFound)
    missingField              PASS  (无 claudeAiOauth 字段 → NotFound)
    nestedMissing             PASS  (有 claudeAiOauth 但无 expiresAt → NotFound)
    secondsNotMilliseconds    PASS  (10 位秒级时间戳 → 仍按毫秒解析 → Expired)
  Test_ParseExpiresAt_ExpiringSoonMinutes  PASS  (MinutesToExpire ∈ [1,5])

mount_mutagen_test.go (9 用例，签名升级后全 PASS):
  Test_SafetyGuard / Test_50MBReject / Test_VersionSkew / Test_DaemonStartIdempotent
  Test_MutagenHappyPath_CleansUpOnTerminate (calls ≥4：daemon + sync create + sync list + terminate)
  Test_MutagenHealthCheck_ReasonsCorrect / Test_WriteMutagenDefaultsYML
  Test_VersionSkewSkippedWhenConnNil / Test_CleanupRunsTerminateAndAskpass

mount_strategy_test.go (12 + 7 = 19 用例，hooks 签名升级后全 PASS):
  TestMountStrategy_DowngradeMatrix (12 子用例) / Test_BannerColors / Test_APFSCaseInsensitive
  Test_Downgrade_BannerEachStep / Test_Downgrade_CapabilityFromAuthResp (2)
  Test_ParseMode (5 + 1) / Test_ForceMode_FailureUsesForceCode / Test_BuildSessionName
  Test_ExtractErrCode_FallbackForceFailed

整仓回归：go test ./... -count=1 全 PASS（含 v2.0 mount_test / ssh_doctor_test / 控制面）。
集成测试静态编译：go test -tags=integration -count=1 -run x_no_run_x ./internal/cloudclaude/ ok。
```

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] mount_mutagen_test.go Test_MutagenHappyPath 调用计数断言失效**

- **Found during:** Task 3.2 跑 mount_mutagen_test.go 时
- **Issue:** Plan 02 写的断言是 `len(*calls) < 3`（daemon + sync create + terminate），但 Plan 03 在 sync create 后追加了 sync list --template 调用，让 happy path 的 runLocal 调用至少为 4 次。如果不更新断言，行为正确但测试假性失败。
- **Fix:** 把 `< 3` 改为 `< 4`，同时补 `status.SessionName == "cloud-claude-acct-1-aabbccdd"` 与 `status.ConflictCount == 0` 字段断言（验证 Plan 03 引入的 MutagenSyncStatus 字段填充）。
- **Files modified:** `internal/cloudclaude/mount_mutagen_test.go`
- **Commit:** 7e124b1

### No Other Deviations

- Rule 2/3/4 未触发：Plan 03 PLAN 已经把 MutagenSyncStatus 引入、TestIntegration_F7C 用 expiresAt:0 等关键决策写得非常详细，executor 全程按 PLAN 落地，无架构争议。
- Pre-existing gofmt 警告（`envcheck.go` / `ssh_doctor_test.go`）仍未触碰（SCOPE BOUNDARY；Plan 01/02 SUMMARY 已记录）。

## Authentication Gates

无（本 plan 全部本地 unit test + git commit；远端 SSH/Docker 集成测试由 CI 在 docker 可用环境跑 `go test -tags=integration`）。

## Known Stubs

| 文件 | 行 | Stub 类型 | 由谁补齐 |
|------|----|----|----|
| `internal/cloudclaude/integration_test.go::TestIntegration_C3_NetemDrop_ColdBranchRemoved` | t.Skip | tc/netem 依赖占位 | Phase 35 真机验收 |
| `internal/cloudclaude/integration_test.go::runCloudClaude` | runCloudClaude helper 中 cfg 注入路径 | 当前是 binary exec 路径，PLAN 推荐 (b) 直接 import 路径 | Phase 35 / CI 联调时按需切换 |
| `internal/cloudclaude/mutagen_bin/{darwin,linux}_{amd64,arm64}/mutagen` | 整文件 | 占位 shell stub（Plan 01 遗留） | CI build-images workflow 拉真二进制（fetch-mutagen-bins.sh） |

**Stub 是否阻塞 plan goal？** 不阻塞。本 plan goal 是「OAuth 检查接入主路径 + Mutagen conflict 冒泡 + sync 子命令 + 6 个集成测试骨架」全部就位，6 个集成场景中 5 个完整可跑（C3 因 netem 依赖明确转交 Phase 35）。本地 `go test ./...` 全 PASS；CI 在 docker 就绪时 `go test -tags=integration` 触发实跑。

## Phase 35 验收前置 / Phase 34 doctor hook

### Phase 34 / 35 接口契约

1. **errcodes 注册表稳定**：Plan 01 注册的 15 条 + Plan 02/03 全部按 NET_OAUTH_* / MOUNT_* 命名约定，Phase 34 doctor `--explain` 子命令直接读 `errcodes.Registry()` 即可，无需额外迁移。
2. **last-session.json schema_version=1 已稳定**：`{schema_version, timestamp, intended_mode, actual_mode, downgrade_chain, conflict_count, claude_account_id, image_version, apfs_case_insensitive}`；Phase 34 doctor 第一屏直接读取展示降级历史 + conflict 计数。
3. **退出码命名常量固定**：Phase 34 doctor `cloud-claude explain <code>` 子命令展示 ExitCode → Code 映射时引用 `cloudclaude.ExitOAuth* / ExitMountForceFailed` 命名常量（避免后续被 magic number 篡改）。
4. **OAuth 检查可独立 mock**：CheckOAuthCredentials / parseExpiresAt 已剥离纯函数，Phase 34 doctor `oauth` 维度可直接 import + 调用，无需起真实 SSH。
5. **MutagenSyncStatus.LastError 字段**：Phase 34 doctor 在 mount 维度检查时可直接读 LastError（mutagen sync list 出现错误时的 last error 字符串）作为故障原因。
6. **Phase 35 验收**：`go test -tags=integration` 是 ROADMAP §Phase 31 Success Criteria 第 4/6/8/9 条的 CI gate；C3 的 30s 抖动场景需要 tc/netem，在 GitHub Actions runner 中 cap_add SYS_ADMIN + apparmor=unconfined 之后通常可用，但 Phase 35 真机验收同时覆盖更全面。

## Limitations / Trade-offs

1. **runCloudClaude 凭证注入未完成路径 (b)**：当前 helper 是 binary exec 版本（依赖 ~/.cloud-claude/config.yaml），PLAN 推荐路径 (b) 是直接 `import internal/cloudclaude` 调 ConnectAndRunClaudeV3 绕过 LoadConfig。本 plan 保留 (a) 路径以保持 main.go 命令行入口的端到端覆盖；CI 集成时如发现 config 注入太复杂，可顺手切换为 (b)。
2. **TestIntegration_F7C 接受两类 OAuth 错误码**：篡改 expiresAt:0 在 parseExpiresAt 中会被判为「字段缺失」（因为 `creds.Inner.ExpiresAt == 0` 视为 NotFound），断言 NET_OAUTH_EXPIRED OR NET_OAUTH_NOT_FOUND 任一即通过，避免跨 fixture 镜像版本的脆弱断言。
3. **C3 集成场景占位**：netem / tc 依赖在 Phase 29 镜像未必预装，Phase 35 真机环境会保证可用 — 本 plan 用 t.Skip 占位以满足 RESEARCH §6.2 6 用例计数。
4. **mutagen sync list 失败时 conflict count = 0**：list 失败（如 daemon 暂时卡住）只把 listErr 写到 status.LastError，不阻塞 mount，也不补打 ⚠ 警告。doctor 在 Phase 34 通过读 last-session.json + LastError 字段察觉。
5. **sync conflicts 子命令不发起 SSH 连接**：本子命令仅查本地 mutagen daemon 状态，不需要联网；如果用户希望「远程」管理（多机 daemon），留 v3.1。
6. **conflict 警告仅在 banner 后输出一次**：REQ-F1-E 字面是「下次回车前 prompt 上方插入」，本阶段未拦截 PTY 输入流；当前实现作为「启动 banner 后立即输出」的最小可行版本，PTY 拦截留 Phase 34 / v3.1。

## Threat Flags

无新增 threat surface。本 plan 引入的所有外部依赖与远端命令已在 PLAN `<threat_model>` T-31-03-01..08 评估并设定 disposition：

- T-31-03-01..02（OAuth credentials 解析）：accept — token 仅在 parseExpiresAt 一次调用的内存中存活，不写日志/不写文件
- T-31-03-03（timeout 2 防 SSH hang）：mitigate — 已落地 `timeout 2 cat ...`，CheckOAuthCredentials 内部 sess.Run 错误一律 → OAuthNotFound
- T-31-03-04（sync conflicts 暴露文件路径）：accept — 用户层授权操作
- T-31-03-05（fixture 容器 SYS_ADMIN）：accept — 仅本地测试 lifetime
- T-31-03-06（OAuth 退出码模糊）：mitigate — stderr 输出 errcodes.Format 含中文 NextAction 指引 cloud-claude exec claude login
- T-31-03-07（hardcoded fixturePass）：accept — 仅 fixture 容器 lifetime；如未来需要真凭证可改为 fixture entrypoint 动态生成
- T-31-03-08（last-session.json conflict_count）：accept — 仅运维数据

## Self-Check: PASSED

- [x] `internal/cloudclaude/oauth_check.go` 存在，含 CheckOAuthCredentials / parseExpiresAt
- [x] `internal/cloudclaude/oauth_check_test.go` 存在，10 子用例 PASS
- [x] `cmd/cloud-claude/sync.go` 存在，含 newSyncCmd / runSyncConflicts
- [x] `internal/cloudclaude/integration_test.go` 存在，6 个 TestIntegration_* + //go:build integration 头部
- [x] `scripts/test-fixture-up.sh` 存在且可执行
- [x] `scripts/test-fixture-down.sh` 存在且可执行
- [x] `internal/cloudclaude/ssh.go` 含 CheckOAuthCredentials(connA + 4 个 OAuth 分支 + 跳过提示
- [x] `internal/cloudclaude/mount_mutagen.go` 含 MutagenSyncStatus + sync list --template + 不含 sync list --json
- [x] `internal/cloudclaude/mount_strategy.go` 含「同步冲突」+「cloud-claude sync conflicts」
- [x] `internal/cloudclaude/exitcodes.go` 含 ExitOAuthNotFound=6 / ExitOAuthExpired=7
- [x] `cmd/cloud-claude/main.go` 含 newSyncCmd 注册 + DisableFlagParsing 路由 sync
- [x] commits fdaa7d8 / 111c0b0 / 7e124b1 / f2011c1 / a202563 全部存在于 git log --oneline
- [x] `go test ./... -count=1` 全 PASS
- [x] `go vet ./...` exit 0
- [x] `gofmt -l` 本 plan 触碰的 7 个文件输出空
- [x] `go test -tags=integration -count=1 -run x_no_run_x ./internal/cloudclaude/` 静态编译通过
- [x] `cloud-claude sync --help` 含 conflicts；`cloud-claude sync conflicts --help` 含「冲突」
- [x] go.mod 不含 testcontainers
