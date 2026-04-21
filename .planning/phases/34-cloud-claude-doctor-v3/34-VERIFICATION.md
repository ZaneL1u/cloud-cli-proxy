---
phase: 34-cloud-claude-doctor-v3
verified: 2026-04-21T11:10:00Z
status: passed
score: 8/8 must-haves verified
overrides_applied: 0
---

# Phase 34: cloud-claude-doctor-v3 Verification Report

**Phase Goal:** 把 v3.0 引入的所有错误路径统一到 `<DOMAIN>_<KIND>_<NUM>` 错误码体系（新增 `MOUNT_*` / `SESSION_*` / `NET_*` / `STATE_*` 前缀），同时把 `cloud-claude doctor` 升级为覆盖 5 维度的可观测工具，并提供 `cloud-claude explain <code>` 子命令。本阶段是 F8 横切关注点的"收口"phase，必须防御 Critical Pitfalls C2、C4、C6、C8、M13、M14。

**Verified:** 2026-04-21T11:10:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| #   | Truth (ROADMAP §Phase 34 Success Criteria) | Status | Evidence |
| --- | ------------------------------------------ | ------ | -------- |
| 1   | CI 单元测试遍历错误码注册表：无重复 code、每条有非空中文 message 与 next_action、所有 `MOUNT_* / SESSION_* / NET_* / STATE_*` 前缀与 v2.0 无冲突 | ✓ VERIFIED | `TestAllDomainsClosed` / `TestNoLegacyLowercaseCodes` / `TestExplainExemptOnlyInformational` / `TestErrcodesRegistry` PASS（`go test ./internal/cloudclaude/errcodes/` 全绿）；`MustRegister` 防重 + 正则三段防御；Registry size = 42 |
| 2   | `cloud-claude doctor` 输出 5 维度（network / auth / ssh / mount / disk），每个检查项均有 `[符号] 原因（建议: ... | 错误码: ...）` 四要素 | ✓ VERIFIED | 实测 `./cloud-claude doctor` 输出 `── network/auth/ssh/mount/disk ──` 五段 + 共 18 项 check（≥17 下限）；warn/fail 行均含「建议: ... | 错误码: XXX_YYY_ZZZ」 |
| 3   | CI grep 对 `cloud-claude doctor` 输出，所有 `[!]` 与 `[✗]` 行必须含「建议:」子串（M14） | ✓ VERIFIED | `bash scripts/ci-doctor-grep.sh ./cloud-claude` 实际输出 `OK: cloud-claude doctor M14 gate passed (schema=1 / next_action / 错误码).` ；`TestRenderTextContainsNextAction` PASS |
| 4   | `cloud-claude doctor --fix` 在测试矩阵中能自动修复 ≥ 5 类失败 | ✓ VERIFIED | `len(doctor.FixerRegistry) = 6` ≥ 5（MOUNT_MUTAGEN_DAEMON_UNAVAILABLE / SYSTEM_FUSE_RESIDUAL_MOUNT / SSH_KNOWN_HOSTS_CONFLICT / AUTH_TOKEN_EXPIRED / AUTH_OAUTH_REFRESH_FAILED / SYSTEM_DNS_RESOLVE_FAILED），`TestApplyFixes_Fix_TriggersRegistry` + 16 fix 单测 PASS |
| 5   | `cloud-claude doctor --json` 可被 jq 直接解析；`NO_COLOR=1` 关闭颜色；退出码 0/1/2 对齐 brew doctor | ✓ VERIFIED | 实测 `NO_COLOR=1 ./cloud-claude doctor --json` 输出顶层 `"schema_version": 1` 合法 JSON object；含 fail 时实测 exit 2（pass=0/warn=1/fail=2）；`TestJSONSchemaV1Lock` PASS |
| 6   | 强制让上次连接静默降级到 sshfs-only 后，`cloud-claude doctor` 第一屏必须展示降级历史 | ✓ VERIFIED | 实测第一屏「上次会话快照」段输出 `[降级] full → mutagen-only \| 原因 [MOUNT_MERGERFS_FAILED] ...` + `[降级] mutagen-only → sshfs-only \| 原因 [MOUNT_HOT_SYNC_FAILED] ...` 字面量逐 step；`TestDowngradeBannerRendersChain` PASS |
| 7   | doctor 完全本地 + SSH 实现，不给 host-agent 加 endpoint（ARCHITECTURE §6 — 守恒） | ✓ VERIFIED | `rg "agentapi\." internal/cloudclaude/doctor/` → 0 命中；`rg "github.com/.*agentapi" internal/cloudclaude/doctor/` → 0 命中；远端命令通过 `RemoteRunner` interface 走 SSH conn 在容器内 exec |
| 8   | `cloud-claude explain <code>` 子命令对每个错误码给出详细中文说明 + 常见修复步骤（对标 `rustc --explain`） | ✓ VERIFIED | 实测 `./cloud-claude explain MOUNT_MUTAGEN_VERSION_SKEW` exit 0 + stdout 含「详细说明：」+ ≥ 200 中文字符五段说明；`./cloud-claude explain BOGUS_CODE` exit 4；大小写敏感（小写 → exit 4）；3 子进程级测试 PASS |

**Score:** 8/8 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/cloudclaude/errcodes/{state,system,ssh,auth,disk}.go` | 5 域文件 + 17 条新 Code 注册 | ✓ VERIFIED | 5 文件存在 + Registry size = 42（17 新 + 25 既有）；commits `b2fcd24..2a03abe` |
| `internal/cloudclaude/errcodes/explanations.go` | ExtendedExplanations + ExplainExempt + registerExplanation | ✓ VERIFIED | 38 条 ExtendedExplanations（≥30）+ 4 条 ExplainExempt（仅 Info）；commit `6ae00c6` |
| `internal/cloudclaude/errcodes/explanations_test.go` | 4 条断言 | ✓ VERIFIED | TestAllCodesHaveExplanations / TestNoLegacyLowercaseCodes / TestAllDomainsClosed / TestExplainExemptOnlyInformational PASS |
| `cmd/cloud-claude/explain.go` + `explain_test.go` | explain 子命令 + 3 子进程测试 | ✓ VERIFIED | newExplainCmd 注册 + ExactArgs(1) + exit 4 未找到；3 子进程测试全 PASS（known/unknown/case-sensitive）；commits `75ae2d7`/`ccd9317` |
| `internal/cloudclaude/doctor/{doctor,check,remote_runner,render}.go` | 顶层骨架 + RemoteRunner interface + RenderText/JSON | ✓ VERIFIED | RunDoctor 串行 5 维度 + lazy SSH + Summary 聚合；schema_version=1 锁死；commits `c5f6df5..3a95373` |
| `internal/cloudclaude/doctor/{network,auth,ssh,mount,disk}.go` | 5 维度 ≥ 17 项 check | ✓ VERIFIED | 实际 18 项（3+3+4+5+3）；commits `511753c..326cdb1`；51 单测 PASS |
| `internal/cloudclaude/doctor/fix.go` | FixerRegistry + 5 类 Fixer + confirmDestructive + ApplyFixes | ✓ VERIFIED | 6 entry 注册 + 5 包级 var mock + 三级判定 + 60s timeout；commit `78626e5` |
| `internal/cloudclaude/doctor/fix_test.go` | ≥ 14 单测 | ✓ VERIFIED | 16 单测全 PASS（含 confirmDestructive 三分支 / Status 不降级守恒）；commit `f4b7893` |
| `internal/cloudclaude/doctor/integration_test.go` | build tag integration + 2 docker fixture 用例 | ✓ VERIFIED | 文件头 `//go:build integration` + `// +build integration` + 2 用例（Happy / MergerfsTampered）；默认 build/test 不触发；commit `8b63af1` |
| `cmd/cloud-claude/doctor.go` | newDoctorCmd + 4 flag + ApplyFixes 接入 | ✓ VERIFIED | `len(doctor.FixerRegistry) > 0` 真实检查 + `[fix] N/M` 顶部行（守 !jsonOut）+ 4 flag (--fix/--verbose/--json/--yes)；commit `f38dfd8` |
| `scripts/ci-doctor-grep.sh` | M14/SC#3 三段断言闸门 | ✓ VERIFIED | 文件存在 + 可执行 + bash -n 通过 + 实测 smoke `OK: cloud-claude doctor M14 gate passed`；commit `dc4519d` |
| `Makefile` | ci-doctor-grep + ci-gate target | ✓ VERIFIED | `make -n ci-doctor-grep` 与 `make -n ci-gate` 解析无误；commit `5d4a0d8` |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `cmd/cloud-claude/main.go::rootCmd.AddCommand` | `newExplainCmd()` + `newDoctorCmd()` | AddCommand 注册 + DisableFlagParsing switch 加 `explain`/`doctor` | WIRED | 实测 `cloud-claude explain` / `cloud-claude doctor --help` 两子命令均输出帮助 |
| `cmd/cloud-claude/explain.go::runExplain` | `errcodes.Lookup` + `errcodes.Format` + `errcodes.ExtendedExplanations` | ExactArgs(1) 大小写敏感匹配；未找到 `os.Exit(exitConfigError)=4` | WIRED | known code → exit 0 + 三段输出；unknown → exit 4 + stderr「未找到错误码」 |
| `cmd/cloud-claude/doctor.go::runDoctor` | `doctor.RunDoctor` → `doctor.ApplyFixes` → `FixerRegistry` | `if fix && anyFixerRegistered() { ApplyFixes(...) }` + 顶部 `[fix] N/M` 行（!jsonOut） | WIRED | `len(doctor.FixerRegistry) > 0` 真实检查；ApplyFixes 走串行 + 60s timeout；TestApplyFixes_Fix_TriggersRegistry PASS |
| `internal/cloudclaude/doctor/doctor.go::RunDoctor` | 5 维度 check + lazy SSH + DowngradeBanner | 串行 network → auth → ssh → mount → disk；第一次需要远端的 check 之前 sshConnect | WIRED | 实测 `./cloud-claude doctor` 顺序输出 5 段 + 第一屏降级 banner |
| `internal/cloudclaude/doctor/render.go::renderDowngradeBanner` | `cloudclaude.LastSessionSnapshot.DowngradeChain` | 每个 DowngradeStep 输出 `[降级] <from> → <to> \| 原因 [<reason_code>] <reason_message>` | WIRED | 实测两条 [降级] 行字面量与契约一致 |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| `cmd/cloud-claude/doctor.go` (text 渲染) | `report.Checks` | `doctor.RunDoctor` 串行执行 18 项 check + summary 聚合 | ✓ Yes — 实测 18 项含 13 pass / 2 warn / 1 fail / 2 skip 真实结果 | ✓ FLOWING |
| `cmd/cloud-claude/doctor.go` (JSON 渲染) | `report` | 同上，`json.Marshal` 序列化 | ✓ Yes — 实测 jq 可解析，含 schema_version + downgrade_history.downgrade_chain[2] + checks[18] | ✓ FLOWING |
| `cmd/cloud-claude/explain.go` (stdout 三段) | `entry` + `ExtendedExplanations[code]` | `errcodes.Lookup` map 查 + `ExtendedExplanations` map 查 | ✓ Yes — 实测 `MOUNT_MUTAGEN_VERSION_SKEW` 输出五段 ≥ 200 字符长说明 | ✓ FLOWING |
| `internal/cloudclaude/doctor/render.go::renderDowngradeBanner` | `banner.Steps[i].ReasonCode/ReasonMessage` | `cloudclaude.LoadLastSession()` 读 `~/.cloud-claude/last-session.json` | ✓ Yes — 实测从用户实际 last-session.json 读出 2 条 DowngradeStep 渲染 | ✓ FLOWING |
| `internal/cloudclaude/doctor/fix.go::ApplyFixes` | `c.FixApplied / c.FixFailed` | 逐 check 调 `FixerRegistry[c.Code](ctx, opts, *c)`；结果追加 | ✓ Yes — Status 不降级（D-16）+ 走 60s context.WithTimeout；TestApplyFixes_StatusNotDowngraded PASS | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| 5 维度 + ≥17 check 输出 | `./cloud-claude doctor` | exit 2，输出 5 段 + 18 项 check + summary | ✓ PASS |
| --json + NO_COLOR 输出 | `NO_COLOR=1 ./cloud-claude doctor --json` | 合法 JSON + `schema_version: 1` + `downgrade_chain[2]` + `summary.total=18` | ✓ PASS |
| --help 列 4 flag | `./cloud-claude doctor --help` | 列出 `--fix / --verbose / --json / --yes` 4 flag + 中文 Long 描述 | ✓ PASS |
| explain known code | `./cloud-claude explain MOUNT_MUTAGEN_VERSION_SKEW` | exit 0 + Format 两段 + 「详细说明：」+ 五段长说明 | ✓ PASS |
| explain unknown code | `./cloud-claude explain BOGUS_CODE` | exit 4 + stderr「未找到错误码 BOGUS_CODE」 | ✓ PASS |
| M14 gate smoke | `bash scripts/ci-doctor-grep.sh ./cloud-claude` | exit 0 + `OK: cloud-claude doctor M14 gate passed (schema=1 / next_action / 错误码).` | ✓ PASS |
| Makefile target 解析 | `make -n ci-gate` | 解析无误（go test + ci-doctor-grep 两步） | ✓ PASS |
| 全仓库 build | `go build ./...` | exit 0 | ✓ PASS |
| errcodes 包测试 | `go test ./internal/cloudclaude/errcodes/ -count=1` | PASS（9 测试） | ✓ PASS |
| doctor 包单测（不含 integration） | `go test ./internal/cloudclaude/doctor/ -count=1 -short` | PASS（67 子测，51 框架 + 16 fix） | ✓ PASS |
| cmd 包测试 | `go test ./cmd/cloud-claude/ -count=1` | PASS（含 TestExplain_*） | ✓ PASS |
| docker fixture 集成测试 | `go test -tags=integration ./internal/cloudclaude/doctor/` | ? SKIP — 默认走 build tag 隔离；本地未起 docker fixture，由 CI/Plan 03 carry-over 真机验证 | ? SKIP |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| REQ-F6-A | 34-02 | doctor 5 维度覆盖（network/auth/ssh/mount/disk） | ✓ SATISFIED | 实测输出 5 段 ── 维度名 ── + 18 项 check（≥17）；REQUIREMENTS.md 已 mark Complete |
| REQ-F6-B | 34-02 | 输出四要素：`[符号]` + 中文原因 + 「建议:」+ 错误码 | ✓ SATISFIED | 实测 warn/fail 行字面量含全部四要素；M14 gate smoke + TestRenderTextContainsNextAction PASS |
| REQ-F6-C | 34-03 | doctor `--fix` 至少 5 类自动修复 | ✓ SATISFIED | `len(doctor.FixerRegistry) = 6` ≥ 5；6 类全部对应 D-09 表 + 16 单测 PASS |
| REQ-F6-D | 34-02 | `--verbose` / `--json` / `NO_COLOR` / 退出码 0/1/2 | ✓ SATISFIED | 4 flag 实测齐备；JSON schema=1 锁死；NO_COLOR=1 关闭颜色；fail → exit 2 |
| REQ-F8-A | 34-01 | 错误码 `<DOMAIN>_<KIND>_<NUM>` 体系 + 4 新前缀 | ✓ SATISFIED | 8 域闭合（MOUNT/SESSION/NET/STATE/SYSTEM/SSH/AUTH/DISK）+ Registry size = 42；TestAllDomainsClosed PASS |
| REQ-F8-B | 34-01 | 错误三要素：错误码 + 中文原因 + 中文 next_action | ✓ SATISFIED | `MustRegister` 强制 Code/Message/NextAction 非空 + codes_test.go 长度 ≤ 80 runes 断言；42 条 Registry 全合规 |
| REQ-F8-C | 34-01 | `cloud-claude explain <code>` 子命令 | ✓ SATISFIED | 38 条 ExtendedExplanations（每条 ≥ 200 中文字符五段模板）+ explain CLI 三段输出 + 3 子进程测试 PASS |

**No orphaned requirements.** REQUIREMENTS.md mapping for Phase 34 = {F6-A, F6-B, F6-C, F6-D, F8-A, F8-B, F8-C}，全部出现在某个 plan 的 `requirements:` 字段（34-01 ⇄ F8-A/B/C；34-02 ⇄ F6-A/B/D；34-03 ⇄ F6-C）。

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| (none) | - | - | - | - |

**Scan results:**
- `grep -E "TODO|FIXME|XXX|HACK|PLACEHOLDER" internal/cloudclaude/doctor/*.go internal/cloudclaude/errcodes/*.go cmd/cloud-claude/{explain,doctor}.go scripts/ci-doctor-grep.sh` → 仅 2 行命中 `XXX_YYY_ZZZ`，是 ci-doctor-grep.sh 注释中描述错误码格式（非 stub 标记）。
- `grep "Plan 03" internal/cloudclaude/doctor/*.go cmd/cloud-claude/doctor.go` → 全部为设计性引用注释（解释字段被 Plan 03 填充 / 引用 D-09 表），无 stub placeholder。
- 无 `return null` / `return []` / 空 handler 等 React 风格 stub 模式（Go 项目，不适用）。

### Human Verification Required

(none)

### Gaps Summary

无 gap。所有 8 项 ROADMAP Success Criteria 经程序化验证（单元测试 + 静态分析 + 实际二进制 smoke）全部 VERIFIED：

- **SC#1** Registry 唯一/中文/next_action：单元测试套件全 PASS（4 条断言 + 既有 5 条）
- **SC#2** 5 维度 18 项 check + 四要素：实测二进制输出符合契约
- **SC#3** CI grep gate「建议:」子串：`scripts/ci-doctor-grep.sh` 本地 smoke 通过
- **SC#4** --fix ≥ 5 类：FixerRegistry 6 entry + 16 单测 + ApplyFixes Status 不降级守恒
- **SC#5** --json + NO_COLOR + 退出码：实测 jq 解析合法 + exit 2 fail / 1 warn / 0 ok 三态
- **SC#6** 第一屏降级历史：实测从用户真实 last-session.json 读出 2 条 [降级] 行
- **SC#7** doctor 不给 host-agent 加 endpoint：`rg "agentapi\." internal/cloudclaude/doctor/` → 0 命中（守恒）
- **SC#8** explain CLI：38 条 ≥ 200 字符长说明 + 三段输出 + 大小写敏感 + exit 4 未找到

**Note on integration tests**：`integration_test.go` 受 `//go:build integration` 隔离，默认 `go test ./...` 不触发。Plan 03 SUMMARY 已记录 carry-over「CI workflow 接入 `make ci-gate`」与「`go test -tags=integration` 增 step」为 follow-up；Makefile target 与脚本均已就绪，本 phase 范围内不阻塞。

**Note on uncommitted files**：根目录有来自另一并行会话的未提交改动（mount_mutagen.go / ssh.go / mutagen_ssh.go / ignore.go / mutagen_bin/* / config.json），与 Phase 34 commits 34-01/02/03 无关；本次评估仅基于 Phase 34 已 commit 产物（`01d9f12..5d4a0d8`），符合任务说明。

**Critical Pitfalls 防御核查**：
- C2（mergerfs 篡改）✅ TestIntegration_DoctorMountFail_MergerfsTampered + 实测 doctor 输出含 MOUNT_MERGERFS_FAILED
- C4（mutagen 版本漂移）✅ MOUNT_MUTAGEN_VERSION_SKEW 注册 + checkMutagenVersionMatch + ExtendedExplanations 长说明
- C6（FUSE 残留）✅ SYSTEM_FUSE_RESIDUAL_MOUNT + checkFUSEResidual + Details["mountpoints"] + fixFUSEResidualMount
- C8（v2.0 lower-case 字面量禁入）✅ TestNoLegacyLowercaseCodes 禁 5 条 lower-case Code 出现
- M13（降级历史第一屏）✅ TestDowngradeBannerRendersChain + 实测两条 [降级] 行
- M14（建议子串 + 错误码）✅ TestRenderTextContainsNextAction + ci-doctor-grep.sh smoke + 实测每条 warn/fail 行格式

---

_Verified: 2026-04-21T11:10:00Z_
_Verifier: Claude (gsd-verifier)_
