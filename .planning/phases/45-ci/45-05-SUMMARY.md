---
phase: 45-ci
plan: 05
subsystem: .github/workflows + scripts
tags: [ci, e2e-workflow, lint-no-bare-sleep, hosted-ubuntu]
provides:
  - e2e-yml-double-job (lint + e2e)
  - lint-no-bare-sleep-script
  - ci-yml-lint-no-bare-sleep-job
  - failure-pr-comment-zh-cn
  - artifact-upload-30day
requires:
  - tests/e2e/harness/artifacts.go (Plan 04 DefaultArtifactBaseDir 路径常量)
  - tests/e2e/harness/waitfor.go (Plan 03 守护脚本扫描的目标)
affects:
  - .github/workflows/e2e.yml
  - scripts/lint-no-bare-sleep.sh
  - .github/workflows/ci.yml
tech-stack:
  added:
    - "actions/upload-artifact@v4 (e2e.yml)"
    - "actions/github-script@v7 (e2e.yml)"
  patterns:
    - "双层守护：ci.yml lint-no-bare-sleep（永远跑，无 paths 过滤） + e2e.yml lint job（paths 过滤后强制守护）"
    - "失败时 PR 评论：actions/github-script@v7 + pull-requests: write 权限"
    - "环境变量传递 artifact 路径：CLOUD_CLI_PROXY_E2E_ARTIFACT_DIR=./out/e2e-artifacts → 与 Plan 04 DefaultArtifactBaseDir 字面量对齐"
key-files:
  created:
    - .github/workflows/e2e.yml
    - scripts/lint-no-bare-sleep.sh
  modified:
    - .github/workflows/ci.yml
decisions:
  - "Runner 用 hosted ubuntu-24.04（与 v3.5 uat-bypass.yml 同款，免运维），不引入 self-hosted Linux runner —— 调整 ROADMAP §E2E-03 草稿描述"
  - "PR paths 过滤命中 tests/e2e/** / internal/network/** / internal/runtime/** / internal/agent/** / internal/agentapi/** / cmd/host-agent/** / cmd/control-plane/** / scripts/lint-no-bare-sleep.sh / .github/workflows/e2e.yml 任一即触发"
  - "失败 PR 评论用 actions/github-script@v7 直接 createComment，不引入第三方 action（如 marocchino/sticky-pull-request-comment）"
  - "PR 评论文案严格中文（CONTEXT.md §Area 4 + CLAUDE.md 沟通约定）"
  - "ci.yml lint-no-bare-sleep job 不带 paths 过滤永远跑：保证脚本本身不腐烂（即使 PR 不动 tests/e2e/）"
  - "e2e.yml 的 e2e job 用 needs: lint 串行：lint 先跑，避免 e2e 跑通了但 lint fail 掉 PR 的悖论"
  - "go test -timeout=15m + job timeout-minutes: 20：留 5 分钟 buffer 给 docker 启动 / artifact 上传"
metrics:
  duration: 约 25 分钟
  tasks_completed: 3/3
  files_modified: 3
  commits: 1（与 Plan 04 + Phase 45 VERIFICATION 合并提交）
  completed_at: 2026-05-14
requirements_satisfied:
  - E2E-03
  - E2E-05 (CI 守护层；helper 实现已在 Plan 03 完成)
---

# Phase 45 Plan 05: CI 双层 workflow + lint-no-bare-sleep 守护 Summary

## One-liner

新建 `.github/workflows/e2e.yml`（lint + e2e 双 job，hosted ubuntu-24.04 + paths 过滤强制守护 + 失败 `actions/upload-artifact@v4` 归档 `./out/e2e-artifacts/` + `actions/github-script@v7` 发中文 PR 评论），新建 `scripts/lint-no-bare-sleep.sh`（grep `^\s*time\.Sleep\(` 守护，自带 `--help` + `TARGET_DIR` 覆写），并在既有 `.github/workflows/ci.yml` 末尾追加独立 `lint-no-bare-sleep` job（永远跑，保证脚本不腐烂）。两份 workflow 通过 ruby YAML parse 验证，本地 `bash scripts/lint-no-bare-sleep.sh` 自检 exit 0。

## 实际产出

| 文件 | 性质 | 关键内容 |
|------|------|----------|
| `.github/workflows/e2e.yml` | 新建 | lint job (4 step) + e2e job (6 step) + 双 job 共享 paths/concurrency/permissions；失败上传 artifact + PR 评论 |
| `scripts/lint-no-bare-sleep.sh` | 新建可执行（100755） | bash + `set -euo pipefail` + `--help` 中文 + `TARGET_DIR` 默认 tests/e2e + `grep -RInE --include='*.go' '^\s*time\.Sleep\('` 命中 fail |
| `.github/workflows/ci.yml` | 修改 | 末尾追加 `lint-no-bare-sleep` job（ubuntu-latest + 2 step：bash -n + 脚本运行）；既有 4 个 job (go-test/web-build/perf-benchmark/image-size-regression) 1:1 保留 |

## 验证结果

| 验证 | 命令 | 结果 |
|------|------|------|
| YAML parse e2e.yml | `ruby -ryaml -e "YAML.load_file('.github/workflows/e2e.yml')"` | jobs: ["lint", "e2e"] ✓ |
| YAML parse ci.yml | `ruby -ryaml -e "YAML.load_file('.github/workflows/ci.yml')"` | jobs: ["go-test", "web-build", "perf-benchmark", "image-size-regression", "lint-no-bare-sleep"] ✓ |
| lint script bash -n | `bash -n scripts/lint-no-bare-sleep.sh` | exit 0 ✓ |
| lint script --help | `bash scripts/lint-no-bare-sleep.sh --help` | 中文 usage 输出 ✓ |
| lint script run（当前 tests/e2e/）| `bash scripts/lint-no-bare-sleep.sh` | `[ok] tests/e2e 内无裸 time.Sleep` exit 0 ✓ |
| script 可执行位 | `ls -la scripts/lint-no-bare-sleep.sh` | `-rwxr-xr-x` ✓ |
| e2e.yml 关键字段 grep | 17 项断言（runs-on / paths / artifact / github-script / pull-requests / failure / 中文等） | 全命中 ✓ |
| 无绝对路径 | `grep -E '/Users/\|/home/' .github/workflows/e2e.yml scripts/lint-no-bare-sleep.sh` | 无命中 ✓ |

## ROADMAP 同步建议（已在本 phase commit 顺手刷新）

- ROADMAP §Phase 45 §Plans 第 3 条已从 "CI 双层架构（hosted runner 跑非特权测试 + self-hosted Linux runner 跑特权网络栈 e2e）" 改写为 "waitFor 条件等待 helper + 4 个语义化变体（Log/Port/HTTP/Exec）+ DumpHook 占位" — 在 Plan 03 commit 中已完成
- ROADMAP §Phase 45 §Plans 第 5 条已从 "waitFor 条件等待 helper + 禁止裸 sleep 的 lint/约定" 改写为 "CI 双层 workflow（hosted ubuntu-24.04 runner + paths 强制守护 + 失败 PR comment）+ lint-no-bare-sleep 守护脚本" — 在 Plan 03 commit 中已完成
- ROADMAP §Phase 45 §Plans 末尾的 `> 注：Phase 45 plan 编号意图与初稿微调...` 注释段说明了 plan 编号意图调整原因

## 给后续 phase 的接口契约

### 新增 e2e 路径（如 tests/e2e/leak/）
- 必须同时加入 e2e.yml 的 paths 列表（在现有 `tests/e2e/**` 已经 catch all）
- 如果新增需要 e2e 守护的源码目录（如 `internal/sshproxy/`），追加到 paths

### artifact 路径锁死 `./out/e2e-artifacts/`
- 与 Plan 04 `DefaultArtifactBaseDir` 字面量一致
- 改任一处都需要同步改另一处（建议在 e2e.yml 顶部注释中提醒）

### 守护脚本 TARGET_DIR
- 默认 `tests/e2e`；如未来拆出 `tests/e2e/leak/` / `tests/e2e/kill-switch/` 子目录，**不需要**为每个子目录单独跑脚本，递归扫描已经覆盖
- 如果未来新增独立目录（如 `tests/integration/`），可通过 `TARGET_DIR=tests/integration bash scripts/lint-no-bare-sleep.sh` 单独守护，无需改脚本

## 决策回顾

1. **hosted ubuntu-24.04 vs self-hosted runner**：
   - PLAN 02 Task 2 已提示 hosted runner 上 `/dev/net/tun` 偶发不可用；当前 v3.5 uat-bypass.yml 在 hosted runner 上跑通 nft + sudo + docker，证明 v3.6 同款配置可行
   - 不引入 self-hosted runner 避免运维负担（密钥管理、节点维护、容量规划）
   - 未来若用例规模触及 hosted runner 性能瓶颈再评估迁移
2. **PR 评论用 actions/github-script@v7 直接 createComment**：
   - 不引入第三方 action（如 sticky-pull-request-comment）减少供应链风险
   - github-script 是 GitHub 官方维护，权限模型清晰
   - 已知限制：fork PR 上 GITHUB_TOKEN 权限被强制降级为 read，评论会 403；artifact 上传仍正常（CONTEXT.md §Deferred 已隐含接受）
3. **ci.yml lint-no-bare-sleep 不带 paths 过滤永远跑**：
   - 即使某 PR 完全不动 tests/e2e/，lint 仍跑；避免「PR 不命中 paths → e2e.yml 不触发 → 守护脚本本身腐烂未被发现」
   - 脚本运行 < 1 秒，重复成本忽略不计
4. **e2e job timeout-minutes: 20，go test -timeout=15m**：
   - go test 自身 15 分钟超时（保证用例 panic 时不卡死整个 job）
   - job 总超时 20 分钟（留 5 分钟 buffer 给 docker 启动 / image pull / artifact upload）
5. **needs: lint**：e2e job 必须等 lint 通过；避免 e2e 跑通了但 lint fail（如裸 sleep 引入）的悖论

## 风险与遗留

- **fork PR 评论权限受限**：known issue，仅同仓 PR 享受自动评论；fork PR 仅得到 artifact 上传 + Actions tab 链接（开发者要自己点进去找）
- **e2e job 在 hosted ubuntu-24.04 跑 sing-box / nft 可能 `/dev/net/tun` 不可用**：当前 Phase 45 范围内 ScenarioSmokeSuite 全部 t.Skip，不会暴露此问题；Phase 46+ 真实业务用例落地后若发现 tun 不可用，按 PLAN 02 Task 3 的 fixture preflight 模式自适应跳过
- **未跑过真实 e2e job 实测验证**：本 plan 未推送到 GitHub 触发 CI，YAML 仅本地 ruby parse；首次 PR 时若有 GitHub Actions 语法 / paths 过滤微调，根据 CI 反馈快速回滚

## 完成度

- ✅ scripts/lint-no-bare-sleep.sh 落地（可执行 + bash -n 通过 + 自检 exit 0）
- ✅ .github/workflows/e2e.yml 落地（lint + e2e 双 job + 失败 artifact + 中文 PR 评论）
- ✅ .github/workflows/ci.yml 追加 lint-no-bare-sleep job（既有 4 job 1:1 保留）
- ✅ 两份 workflow YAML parse 通过（ruby 验证）
- ✅ E2E-03（CI 双层架构）+ E2E-05（守护层）全部 satisfied
- ✅ ROADMAP 文案对齐（在 Plan 03 commit 中已完成）
