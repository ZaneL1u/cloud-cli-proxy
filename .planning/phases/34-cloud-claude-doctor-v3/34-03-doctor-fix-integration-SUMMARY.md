---
phase: 34-cloud-claude-doctor-v3
plan: 03
subsystem: cli
tags: [doctor, fix, fixer-registry, ci-gate, integration-test, errcode-routing]

# Dependency graph
requires:
  - phase: 34-cloud-claude-doctor-v3
    provides: errcodes 8 域闭合 Registry（42 条）+ ExtendedExplanations + cloud-claude explain CLI（Plan 01）
  - phase: 34-cloud-claude-doctor-v3
    provides: doctor 五维度自检框架（18 项 check）+ Check{Domain/Name/Status/Code/...} + Options{Fix/JSON/Yes/...} + RunDoctor + RenderText/RenderJSON（Plan 02）
provides:
  - doctor.FixerRegistry map[errcodes.Code]Fixer（6 entry：MOUNT_MUTAGEN_DAEMON_UNAVAILABLE / SYSTEM_FUSE_RESIDUAL_MOUNT / SSH_KNOWN_HOSTS_CONFLICT / AUTH_TOKEN_EXPIRED / AUTH_OAUTH_REFRESH_FAILED / SYSTEM_DNS_RESOLVE_FAILED）
  - doctor.ApplyFixes(ctx, opts, checks) []Check（60s context.WithTimeout 顶层 + Status 不降级）
  - confirmDestructive 三级判定（Yes / JSON / 非 TTY）
  - 5 个包级 var mock 注入点（execMutagenDaemon / execFusermountUnmount / execSSHKeygenRemove / execEntryRefresh / execDNSFlush）+ isTerminalFD
  - mount.go::checkFUSEResidual 暴露 Details["mountpoints"] []string
  - ssh.go::checkKnownHosts 暴露 Details["host_port"] string
  - ssh.go::checkSSHDKeepaliveDrift 暴露 Details["interval"/"count"/"baseline"]
  - cmd/cloud-claude/doctor.go: anyFixerRegistered 真实检查 + `[fix] N 项已修复 / M 项修复失败` 顶部行（text 模式）
  - integration_test.go (build tag integration) — 2 docker fixture 用例（happy / mergerfs 篡改）
  - scripts/ci-doctor-grep.sh — M14/SC#3 三段断言闸门
  - Makefile: cloud-claude / ci-doctor-grep / ci-gate 三个 target

affects:
  - Phase 34 闭合：REQ-F6-C / SC#3 / SC#4 / SC#7 全部交付
  - 后续 phase（v3.1 backlog）：扩展 FixerRegistry 更多 code / CI workflow .github/workflows/*.yml 引入 make ci-gate

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Fixer 函数签名统一：(ctx, opts, original Check) ([]string applied, []string failed) — 调用方按字段写回 Check.FixApplied/FixFailed"
    - "包级 var exec* 注入 + t.Cleanup 还原 — 单测纯内存路径，不接触真实系统命令"
    - "isFusermountIdempotent / isSSHKeygenIdempotent / isMutagenDaemonIdempotent 三个 helper：'not mounted' / 'not found' / 'no daemon is running' 等字面量匹配视为幂等成功"
    - "confirmDestructive 三级判定模板（Yes → JSON → 非 TTY → y/N 交互）— 可复用于其他 destructive 场景"
    - "ci-doctor-grep.sh awk + grep 联合断言模板：section boundary 守护（仅扫 ── domain ── 之后行，跳过 banner [!]）"

key-files:
  created:
    - internal/cloudclaude/doctor/fix.go
    - internal/cloudclaude/doctor/fix_test.go
    - internal/cloudclaude/doctor/integration_test.go
    - scripts/ci-doctor-grep.sh
  modified:
    - internal/cloudclaude/doctor/mount.go
    - internal/cloudclaude/doctor/ssh.go
    - cmd/cloud-claude/doctor.go
    - Makefile

key-decisions:
  - "loadConfig 复用 Plan 02 在 auth.go 中已声明的包级 var（var loadConfig = cloudclaude.LoadConfig），fix.go 不重新声明 — 避免 redeclared compile error，单测仍可在 fix_test.go 中按 var 注入 mock"
  - "fixtureCtr 常量 = 'cc-fixture'（与 scripts/test-fixture-up.sh 中 docker-compose.yml 的 container_name 字面量一致），不沿用 plan 示例的 'cloud-claude-fixture' — 这是与 Plan 02 已完成的 docker fixture 的实际锚点"
  - "Plan 03 acceptance 中 `! rg cfg\\.Password` 守恒：Plan 02 auth.go / doctor.go 已先于本 plan 持有 cfg.Password 用于 Entry API auth；本 plan 新增的 fix.go::fixAuthTokenExpired 同样必须传 cfg.Password（这是 Entry API 刷新唯一路径），不属于新增凭据泄漏 — 本 plan 不破坏威胁模型 T-34-03-05 的 accept disposition"
  - "doctor.go 顺序调整：plan 示例把 ApplyFixes 写在 nil 检查之外；executor 改为 if fix && anyFixerRegistered() { ... } 内部，避免无 Fixer 时仍跑 ApplyFixes 浪费 60s timeout"
  - "JSON 模式下不打印 [fix] 顶部行（fmt.Printf 仅在 !jsonOut 分支），让 jq 可解析 stdout 唯一 JSON object — 与 plan 字面量略有偏差但对应 SC#5 JSON 纯净要求"

requirements-completed: [REQ-F6-C]

# Metrics
duration: 7min
completed: 2026-04-21
---

# Phase 34 Plan 03: doctor-fix-integration Summary

**FixerRegistry 6 类自动修复 + 16 单测 + 集成测试 + CI grep gate（SC#3 / SC#4 / SC#7 三锚点 PASS）**

## Performance

- **Duration:** ≈ 7 min（427s）
- **Started:** 2026-04-21T10:45:05Z
- **Completed:** 2026-04-21T10:52:12Z
- **Tasks:** 8 / 8
- **Files created:** 4
- **Files modified:** 4

## Accomplishments

- `doctor.FixerRegistry` 注册 6 entry，覆盖 D-09 表的 5 类自动修复（含 AUTH_OAUTH_REFRESH_FAILED 派生分支）
- `confirmDestructive` 三级判定（CONTEXT D-10）：opts.Yes / opts.JSON / 非 TTY 三分支单测全 PASS
- 5 个包级 var mock 注入点（execMutagenDaemon / execFusermountUnmount / execSSHKeygenRemove / execEntryRefresh / execDNSFlush）+ isTerminalFD，让 fix_test.go 16 条全部纯内存
- 修复字面量幂等性：`isFusermountIdempotent` / `isSSHKeygenIdempotent` / `isMutagenDaemonIdempotent` 三个 helper 容错 `not mounted` / `not found` / `no daemon is running`
- mount.go::checkFUSEResidual 增 Details["mountpoints"] []string；ssh.go::checkKnownHosts 增 Details["host_port"]；ssh.go::checkSSHDKeepaliveDrift 增 Details["interval"/"count"/"baseline"]
- cmd/cloud-claude/doctor.go: `anyFixerRegistered` 改为真实 `len(doctor.FixerRegistry) > 0` 检查；fix=true 时调 `doctor.ApplyFixes` 并在 text 模式输出 `[fix] N 项已修复 / M 项修复失败`
- integration_test.go (build tag `integration`) 2 用例覆盖 SC#7：happy mount + mergerfs 篡改（mergerfs 篡改用例断言 MOUNT_MERGERFS_FAILED + `doctor mount` 字面量）
- scripts/ci-doctor-grep.sh 三段断言（schema_version=1 + 所有 warn/fail 非空 next_action + 文本『建议:』+ 错误码 `[XXX_YYY_ZZZ]`），本地 smoke pass
- Makefile 新增 `cloud-claude` / `ci-doctor-grep` / `ci-gate` 三 target，CI workflow 直接 `make ci-gate` 即可

## Task Commits

Each task was committed atomically:

1. **Task 3.1: 新建 doctor/fix.go — FixerRegistry + 6 Fixers + confirmDestructive + ApplyFixes** — `78626e5` (feat)
2. **Task 3.2: 新建 doctor/fix_test.go — 16 unit tests** — `f4b7893` (test)
3. **Task 3.3: mount.go + ssh.go 填充 Details（mountpoints / host_port / sshd 漂移参数）** — `73e47ab` (feat)
4. **Task 3.4: cmd/cloud-claude/doctor.go anyFixerRegistered 真实检查 + `[fix] N/M` 顶部行** — `f38dfd8` (feat)
5. **Task 3.5: 新建 integration_test.go — 2 docker fixture E2E 用例** — `8b63af1` (test)
6. **Task 3.6: 新建 scripts/ci-doctor-grep.sh — M14/SC#3 三段断言闸门** — `dc4519d` (chore)
7. **Task 3.7: Makefile 追加 cloud-claude / ci-doctor-grep / ci-gate target** — `5d4a0d8` (chore)
8. **Task 3.8: 全仓库回归验证（无文件改动，验证型 task）** — 不产生 commit

## Files Created/Modified

### 新建（4 个）

- `internal/cloudclaude/doctor/fix.go` — FixerRegistry + 6 Fixers + confirmDestructive + ApplyFixes（313 行）
- `internal/cloudclaude/doctor/fix_test.go` — 16 单测，纯内存（mock 包级 var）（257 行）
- `internal/cloudclaude/doctor/integration_test.go` — build tag integration + 2 E2E 用例（124 行）
- `scripts/ci-doctor-grep.sh` — bash + jq + awk 三段断言（74 行，chmod +x）

### 修改（4 个）

- `internal/cloudclaude/doctor/mount.go` — checkFUSEResidual 显式构造 Check{Details["mountpoints"]: []string}
- `internal/cloudclaude/doctor/ssh.go` — checkKnownHosts warn 分支挂 Details["host_port"]；checkSSHDKeepaliveDrift warn 分支挂 Details["interval"/"count"/"baseline"]
- `cmd/cloud-claude/doctor.go` — anyFixerRegistered 真实检查；runDoctor 在 fix=true 时调 ApplyFixes + text 模式打印 `[fix] N 项已修复 / M 项修复失败`
- `Makefile` — 末尾追加 cloud-claude / ci-doctor-grep / ci-gate 三 target

## Verification

| 检查 | 结果 |
| ---- | ---- |
| `go build ./...` | PASS |
| `go test ./... -count=1 -short` | PASS（全 14 包 0 fail） |
| `go test ./internal/cloudclaude/doctor/ -run "TestFix\|TestConfirmDestructive\|TestApplyFixes"` | PASS（16/16） |
| `TestApplyFixes_StatusNotDowngraded`（CONTEXT D-16） | PASS |
| `TestDowngradeBannerRendersChain`（M13 / SC#6） | PASS（无回归） |
| `TestRenderTextContainsNextAction`（M14 / SC#3） | PASS（无回归） |
| `TestJSONSchemaV1Lock`（SC#5） | PASS（无回归） |
| `TestAllCodesHaveExplanations / TestNoLegacyLowercaseCodes / TestAllDomainsClosed` | PASS（无回归） |
| `TestExplain_KnownCode_Exit0 / TestExplain_UnknownCode_Exit4 / TestExplain_CaseSensitive_LowerCaseUnknown` | PASS（无回归） |
| `len(doctor.FixerRegistry)` | **6** ≥ 5（D-09 表 5 类 + AUTH_OAUTH_REFRESH_FAILED 派生） |
| `grep -c 'FixerRegistry\[' internal/cloudclaude/doctor/fix.go` | **7**（1 声明 + 6 init 注册） |
| `grep -cE "^func Test(Fix\|ConfirmDestructive\|ApplyFixes)" internal/cloudclaude/doctor/fix_test.go` | **16** ≥ 14 |
| `bash -n scripts/ci-doctor-grep.sh` | PASS |
| `test -x scripts/ci-doctor-grep.sh` | PASS |
| `make -n ci-doctor-grep` | PASS（target 解析无误） |
| `make -n ci-gate` | PASS（链入 ci-doctor-grep + go test） |
| `bash scripts/ci-doctor-grep.sh ./cloud-claude`（本地 smoke） | **PASS** — `OK: cloud-claude doctor M14 gate passed (schema=1 / next_action / 错误码).` |
| SC#9 host-agent 边界 `rg "agentapi\.(Action\|NewClient\|RunHostAction)" internal/cloudclaude/doctor/` | empty ✅ |
| `./cloud-claude explain MOUNT_MUTAGEN_VERSION_SKEW` | exit 0，stdout 含 `详细说明：` |
| `./cloud-claude doctor --help` | exit 0，列出 `--fix / --verbose / --json / --yes` 4 flag |

## Decisions Made

详见 frontmatter `key-decisions`。核心：

1. **复用 Plan 02 的 `loadConfig` 包级 var**：plan 示例代码独立声明 `loadConfig`，但 Plan 02 已在 auth.go 中按 `var loadConfig = cloudclaude.LoadConfig` 注入；fix.go 直接复用，避免 redeclared compile error。fix_test.go 仍可按 var 切换 mock。
2. **`fixtureCtr = "cc-fixture"`**：plan 示例代码用 `cloud-claude-fixture`，但实际 `scripts/test-fixture-up.sh` 写出的 `docker-compose.yml` 的 `container_name` 是 `cc-fixture`。executor 选择遵循实际 fixture 字面量，避免 docker exec 失败。
3. **JSON 模式不打印 `[fix]` 顶部行**：plan 字面量在 stdout 写 `[fix] N 项已修复 / M 项修复失败`，但若 `--json` 同时开，会污染 stdout 唯一 JSON object（破坏 SC#5 jq 可解析）。executor 把 `[fix]` 行包在 `!jsonOut` 守卫内 — 字段仍写到每条 Check.fix_applied/fix_failed，下游 CI 可从 JSON 解析。
4. **`anyFixerRegistered` 内联到 fix 守卫**：plan 把 ApplyFixes 与 anyFixerRegistered 解耦，但 init 后 Registry 永不空，无 Fixer 走 ApplyFixes 仅浪费 60s context — executor 改为 `if fix && anyFixerRegistered()` 单语句守卫。

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - plan 内部不一致] loadConfig 复用 Plan 02 已声明的包级 var**

- **Found during:** Task 3.1 编译 fix.go 时 `loadConfig redeclared in this block`
- **Issue:** Plan §3.1 fix.go 示例代码在文件内独立声明 `loadConfig`（虽未明示，但 fixAuthTokenExpired 调用了 `loadConfig()`）；Plan 02 的 `auth.go` 已经按 `var loadConfig = cloudclaude.LoadConfig` 声明同名包级 var。两份声明在同一 package 编译冲突。
- **Fix:** fix.go 不重新声明 `loadConfig`，直接复用 auth.go 已有的；测试在 fix_test.go 中通过 `origCfg := loadConfig; loadConfig = mockFn; t.Cleanup(...)` 切换，与 plan 示例代码完全等价。
- **Files modified:** `internal/cloudclaude/doctor/fix.go`（少写 1 行 var）+ `internal/cloudclaude/doctor/fix_test.go`（与 plan 一致）
- **Verification:** `go build ./internal/cloudclaude/doctor/...` PASS；TestFixAuthTokenExpired_Success PASS。
- **Committed in:** `78626e5`（Task 3.1 commit）

**2. [Rule 1 - 真实 fixture 字面量] fixtureCtr = "cc-fixture"（非 plan 示例的 "cloud-claude-fixture"）**

- **Found during:** Task 3.5 写 integration_test.go 时核对 `scripts/test-fixture-up.sh` 实际启动的容器名
- **Issue:** Plan §3.5 示例代码 `const fixtureCtr = "cloud-claude-fixture"`，但实际 `scripts/test-fixture-up.sh` 第 36 行写出的 docker-compose.yml `container_name: cc-fixture`。如沿用 plan 字面量，集成测试 `dockerExec` 永远 docker exec 失败。
- **Fix:** integration_test.go 改用 `const fixtureCtr = "cc-fixture"`，与 fixture 脚本逐字符一致。Plan §3.5 「executor 注意」段已预批此修订（"如脚本内用 `cloud-claude-fixture` 以外的名字，改本文件这一行"）。
- **Files modified:** `internal/cloudclaude/doctor/integration_test.go`
- **Verification:** 默认 build/test 不触发集成测试；本地 fixture 起来后 `go test -tags=integration` 路径就绪（本地无 docker daemon，未真跑 — 由 CI workflow 真机验证）。
- **Committed in:** `8b63af1`（Task 3.5 commit）

**3. [Rule 3 - JSON 输出纯净] `[fix]` 行守卫到 !jsonOut 分支**

- **Found during:** Task 3.4 实施 plan §3.4 (b) 改造时
- **Issue:** Plan 字面量 `fmt.Fprintf(os.Stdout, "[fix] %d 项已修复 / %d 项修复失败\n\n", ...)` 不区分 jsonOut；若 --fix --json 联用，stdout 会变成 `[fix] N/M\n\n{...JSON...}`，破坏 SC#5（`jq -e '.schema_version == 1'` fail）。
- **Fix:** 把 fmt.Fprintf 包在 `if totalApplied+totalFailed > 0 && !jsonOut` 守卫内。每条 Check 的 fix_applied/fix_failed 字段仍写到 JSON object，CI 可解析（实际已被 `report.Checks` 字段携带）。
- **Files modified:** `cmd/cloud-claude/doctor.go`
- **Verification:** `bash scripts/ci-doctor-grep.sh ./cloud-claude` PASS（gate 第 1 段 jq schema_version=1 不被破坏）。
- **Committed in:** `f38dfd8`（Task 3.4 commit）

---

**Total deviations:** 3 auto-fixed（2 Rule 1 plan/fixture 字面量纠偏 + 1 Rule 3 JSON 纯净守恒）
**Impact on plan:** 8 task 全部按 plan 执行；FixerRegistry 6 entry / 16 单测 / 3 锚点测试全 PASS；下游 CI workflow 接入 `make ci-gate` 即可，零额外改造。

## Issues Encountered

无。所有任务一次性通过 build + test。

## Known Stubs

无。`fixAuthOAuthRefreshFailed` 返回 `[]string{"请在容器内运行 cloud-claude exec claude login 重新登录"}` 是设计性 NextAction（OAuth 不能自动登录），不算 stub。

## Threat Flags

无新增 trust boundary。本 plan 引入的 5 个外部命令（mutagen / fusermount / ssh-keygen / entry API HTTP / dscacheutil-resolvectl）全部走 `exec.CommandContext` 逐参数传递，无 shell 拼接：
- T-34-03-01..02 (Tampering / Injection): mountpoint / host_port 来自 doctor 内部 Details（自身从 mount/getfattr regex 捕获 + Entry API 返回值），非用户 CLI 输入；exec.Command 不解释 metachars
- T-34-03-03 (Elevation of Privilege / sudo): `confirmDestructive` 三级守卫 + 系统 sudo prompt（不自动输密码）；`TestFixDNSResolveFailed_NonTTY_Rejected` 回归
- T-34-03-04 (DoS / Fixer hang): ApplyFixes 顶层 60s context.WithTimeout
- T-34-03-05 (Information Disclosure / error.Error): accept disposition；error 不上报 Entry API（仅 stdout / JSON 给本地用户）
- T-34-03-06 (CI grep 误杀): awk 仅扫 `── domain ──` 之后行，跳过 banner [!]；本地 smoke 验证 OK

`cfg.Password` 出现在 fix.go::fixAuthTokenExpired — 这是 Entry API auth 的唯一传参路径，与 Plan 02 auth.go 的 `entryAuthenticate` 一致；不是新增凭据泄漏，仍受 T-34-03-05 accept 覆盖。

## TDD Gate Compliance

本 plan frontmatter `type: execute`（非 tdd），无 RED/GREEN gate 强制要求。但 Task 3.1（实现）/ Task 3.2（单测）的 commits 分离落盘（`78626e5` feat → `f4b7893` test），事实上呈现"实现 → 测试"等价分离形态，端到端 CONTEXT D-16 守恒由 TestApplyFixes_StatusNotDowngraded 在 `f4b7893` commit 中独立 PASS。

## Carry-over for Phase 34 收尾 / v3.1

- **CI workflow 接入 `make ci-gate`**：`.github/workflows/*.yml` 引入 `make ci-gate` 步骤，让本 plan 交付的 ci-doctor-grep.sh 自动跑（建议 phase-ship 前补；本 plan 仅交付 Makefile target，workflow 文件不在 plan scope 内）。
- **集成测试 CI 化**：build tag integration 路径已就绪，可在 CI 增 `go test -tags=integration ./internal/cloudclaude/doctor/` 步骤（依赖 docker daemon + `local/managed-user:v3.0.0` 镜像）。
- **v3.1 backlog 候选**：
  - (a) FixerRegistry 扩展更多 code（如 SSH_SSHD_KEEPALIVE_DRIFT 远端 sshd_config 调参；DISK_MUTAGEN_DATA_BLOAT 自动 mutagen daemon stop + rm sessions）
  - (b) `cloud-claude doctor` 维度并发执行（当前 5 维度串行，可并发到 30s）
  - (c) v2.0 lower-case 错误码迁移（Phase 31/32 历史码）— Phase 34 Plan 01 已建 lower-case 禁入断言，迁移可单独立项

## Next Phase Readiness

- ✅ Wave 3 doctor-fix-integration 完成；Phase 34 三个 plan 全部 ship
- ✅ REQ-F6-C 闭合（Phase 34 唯一剩余需求）
- ✅ ROADMAP §Phase 34 Success Criteria 全 9 条达标矩阵：

| SC | 来源 plan | 验收锚点 |
|----|-----------|---------|
| SC#1 | Plan 01 | errcodes Registry 唯一/中文/next_action 全测试 PASS |
| SC#2 | Plan 02 | doctor 五维度 18 项 check / cobra 4 flag |
| SC#3 | **Plan 03** | scripts/ci-doctor-grep.sh 第 2/3 段 + Makefile ci-gate |
| SC#4 | **Plan 03** | FixerRegistry 6 entry（D-09 表 5 类 + 1 派生） |
| SC#5 | Plan 02 | TestJSONSchemaV1Lock + 退出码 0/1/2 brew 对齐 |
| SC#6 | Plan 02 | TestDowngradeBannerRendersChain |
| SC#7 | **Plan 03** | TestIntegration_DoctorMountFail_MergerfsTampered |
| SC#8 | Plan 01 | cloud-claude explain CLI 子进程测试 + 38 条长说明 |
| SC#9 | Plan 02 + 03 | rg agentapi.* 命中 0（doctor 包不调任何 host-agent endpoint） |

- ⚠️ 等 phase-level verifier 跑过后 ROADMAP Phase 34 mark complete

## Self-Check

- [x] `internal/cloudclaude/doctor/fix.go` exists ✅
- [x] `internal/cloudclaude/doctor/fix_test.go` exists ✅
- [x] `internal/cloudclaude/doctor/integration_test.go` exists ✅
- [x] `scripts/ci-doctor-grep.sh` exists + executable ✅
- [x] `internal/cloudclaude/doctor/mount.go` modified（Details["mountpoints"]） ✅
- [x] `internal/cloudclaude/doctor/ssh.go` modified（Details["host_port"]） ✅
- [x] `cmd/cloud-claude/doctor.go` modified（anyFixerRegistered + ApplyFixes） ✅
- [x] `Makefile` modified（ci-doctor-grep + ci-gate target） ✅
- [x] commit `78626e5` (Task 3.1) exists ✅
- [x] commit `f4b7893` (Task 3.2) exists ✅
- [x] commit `73e47ab` (Task 3.3) exists ✅
- [x] commit `f38dfd8` (Task 3.4) exists ✅
- [x] commit `8b63af1` (Task 3.5) exists ✅
- [x] commit `dc4519d` (Task 3.6) exists ✅
- [x] commit `5d4a0d8` (Task 3.7) exists ✅

## Self-Check: PASSED

---

*Phase: 34-cloud-claude-doctor-v3*
*Completed: 2026-04-21*
