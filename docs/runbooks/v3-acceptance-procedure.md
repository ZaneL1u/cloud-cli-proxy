# v3.0 验收流程手册（v3.0+）

> 适用版本：v3.0 起；对应阶段 Phase 35（e2e）
> 关联需求：BASE-01..04 / 30 条 functional REQ（REQ-F1..F8） / M5 / M13 / C6
> 主脚本：`scripts/v3-acceptance-checklist.sh`（37 项一键聚合）

---

## 1. 背景

v3.0 把 Phase 29-34 引入的所有 user-visible 能力（mergerfs 三路 mount / tmux 多端 / doctor 五维度 / claude-state 持久化卷 / errcodes 体系）压进 **34 条 REQ + 4 条 BASE 性能基线 + 3 个真机 pitfall** 的统一验收闸门。

本手册指引你在 **三种环境** 中按顺序跑通这条闸门并完成签字，覆盖率与权威口径如下：

| 环境 | 适用 track | 关键真机场景 | 自动 / 人工 |
|------|-----------|-------------|------------|
| CI（GitHub Actions） | base / req-* 中 SKIP 真机的项 | BASE-01 / BASE-04 | 自动 |
| macOS APFS 真机 | all（pitfalls 含 M5） | M5 APFS / BASE-02 / BASE-03 30s 抖动 | 自动 + 签字 |
| Ubuntu 25.04 真机 | all（pitfalls 含 C6） | C6 AppArmor + 三路 FUSE / BASE-03 2min | 自动 + 签字 |

判定口径（人工无主观）：

- **「无感知」** = `pgrep_survived_full_duration=true` AND `buffer_diff_lines=0` AND `token_replayed=true`（来自 Plan 02 `uat-network-resilience.sh` JSON）
- **「三段式进度」** = `stderr_progress_matches=true`（来自 Plan 01 `cold-start-benchmark.sh` JSON）
- **「双向无数据丢失」** = `cat /workspace/Foo.txt` 与 `cat /workspace/foo.txt` 均非空且内容不同

---

## 2. 前置条件

### 2.1 通用

- 仓库 main 分支已合并 Phase 29-34 全部 SUMMARY
- `bash scripts/v3-acceptance-checklist.sh --help` 退出码 0
- `jq` `hyperfine` `ripgrep` 已安装（`brew install jq hyperfine ripgrep` / `apt install jq hyperfine ripgrep`）

### 2.2 CI 环境（GitHub Actions，PR 触发自动）

- `.github/workflows/ci.yml` 已含 `perf-benchmark` + `image-size-regression` 两 job（Plan 04 落地）
- artifact `perf-bench-<sha>` 内含 `.planning/phases/35-e2e/benchmarks/bench-*.json`
- artifact `image-size-<sha>` 内含 `verify-managed-image.sh` 输出

### 2.3 macOS APFS 真机

- Apple Silicon 或 Intel mac、macOS 14+
- 文件系统 APFS（`diskutil info / | grep -i 'File System Personality'` 含 `APFS`）
- Docker Desktop 4.30+（`docker info` 可达）
- 已 `cloud-claude login`，至少有一个 claude_account

### 2.4 Ubuntu 25.04 真机

- 裸机或云主机、内核 ≥ 6.12（`uname -r`）
- AppArmor override 已部署：`/etc/apparmor.d/local/fusermount3` 含 `capability dac_override,`，参 `docs/runbooks/v3-apparmor-deployment.md` §3
- `aa-status | grep fusermount3` 显示已加载
- Docker 25.x+（`docker --version`）

---

## 3. 执行流程

> ⚠️ **安全警告**：M13 三层破坏链（`pkill mergerfs` / `fusermount3 -u` / `pkill mutagen-agent`）必须在 fixture 或 staging 容器执行，**严禁在生产容器跑** `--track=pitfalls --confirm-destructive`。主脚本默认要求 `--confirm-destructive` opt-in 闸门（T-35-05-03 mitigation）。

### 3.1 CI 环境（PR 触发自动）

CI 中两个 job 自动跑，无需手工操作：

```bash
# 手工触发任一最近 PR：
gh workflow run ci.yml --ref <branch>

# 期望两个 ✅：
#   perf-benchmark            ✅
#   image-size-regression     ✅
```

CI 不跑 `--track=all`（环境不全），只覆盖 BASE-01（perf）+ BASE-04（image-size）。其余 track 通过真机环节兜底。

### 3.2 macOS APFS 真机执行

```bash
# 步骤 1：启动 cloud-claude 到目标容器（保持终端 1 活跃）
cloud-claude --mount-mode=auto

# 步骤 2：另开终端 2 跑验收（替换 <ctr> 为 step 1 的容器名）
bash scripts/v3-acceptance-checklist.sh \
  --track=all \
  --env=macos \
  --target-container=<ctr> \
  --report-md=docs/runbooks/v3-acceptance-report-$(date +%Y%m%d).md
```

预期：
- `pitfalls` track 中 **M5 APFS 场景应 PASS**（脚本会本地写 `Foo.txt` + `foo.txt` 双文件，10s 后 `docker exec cat` 双断言）
- `BASE-01 / BASE-02 / BASE-03 30s` 应 PASS
- `BASE-03 2min` / `REQ-F3-C` / `REQ-F3-D` 需手工配合（终端 1 看到「最终失败提示」UI 并签字）
- `REQ-F5-*` / `REQ-F7-B/C/D` 需双端 / OAuth / 删除场景手工配合

### 3.3 Ubuntu 25.04 真机执行

```bash
# 步骤 1：AppArmor override 部署（一次性，参 v3-apparmor-deployment.md）
sudo bash deploy/scripts/host-preflight.sh

# 步骤 2：跑验收
bash scripts/v3-acceptance-checklist.sh \
  --track=all \
  --env=ubuntu25 \
  --target-container=<ctr> \
  --confirm-destructive \
  --report-md=docs/runbooks/v3-acceptance-report-$(date +%Y%m%d).md

# 步骤 3：三路 FUSE 专项（独立断言，C6 已在 step 2 内自动跑）
bash scripts/verify-fuse-compat.sh
```

预期：
- `C6 Ubuntu 25.04` 应 PASS（`host-preflight.sh` 退出 0 + `verify-fuse-compat.sh` 三路全 PASS）
- `M13 三层降级链` 应 PASS（`--confirm-destructive` 已 opt-in）
- 同 §3.2 真机签字项目同步收集

---

## 4. 签字栏模板（Sign-off）

执行结束后主脚本会写入 `docs/runbooks/v3-acceptance-report-YYYYMMDD.md`，自动生成下面这张表的占位行；签字人需补齐 **机器信息 / 执行时间 / 签字人 / 证据**。

### 签字（人工关键场景）

| 场景 | 机器信息 | 执行时间 | 签字人 | 证据 |
|------|---------|---------|--------|------|
| M5 APFS case-insensitive 双向同步 | hostname / OS 版本 / CPU | YYYY-MM-DD HH:MM | @user | `.planning/phases/35-e2e/benchmarks/v3-acceptance-*.json` |
| 2min 拔网自动重连（BASE-03 / REQ-F3-C） | hostname / iface / 断网方式（tc/iptables/物理） | YYYY-MM-DD HH:MM | @user | `.planning/phases/35-e2e/benchmarks/v3-acceptance-*.json` |
| Ubuntu 25.04 AppArmor + 三路 FUSE（C6） | hostname / kernel / ubuntu_version | YYYY-MM-DD HH:MM | @user | `.planning/phases/35-e2e/benchmarks/v3-acceptance-*.json` |

### 签字流程（4 步固定）

1. 在目标环境跑 §3.2 / §3.3 → 主脚本生成 JSON + MD 双报告
2. **必须在 MD 报告中填写机器信息**（hostname / uname / docker --version / cloud-claude --version；macOS 补 `diskutil info /`；Ubuntu 补 `. /etc/os-release`）
3. MD 报告附于 Phase 35 PR description
4. PR 合并 = 签字通过；任何场景 FAIL → 不合并 + 触发 `/gsd-plan-phase 35 --gaps`

### 环境信息收集脚本

```bash
echo "hostname: $(hostname)"
echo "uname:    $(uname -a)"
echo "docker:   $(docker --version 2>/dev/null)"
echo "cloud-claude: $(cloud-claude --version 2>/dev/null)"
# macOS 补充：
diskutil info / | grep -E 'Type|Encrypted|File System Personality'
# Ubuntu 补充：
. /etc/os-release && echo "ubuntu: $VERSION_ID"
uname -r
```

> 🔒 **T-35-05-01 脱敏提示**：开源仓库场景需在 PR 报告中把 hostname 替换为 `macbook-pro-<user>` 等通用占位；私有仓库可保留原值。JSON 默认含完整 uname，按需手工脱敏。

---

## 5. 报告归档

| 路径 | 用途 | 是否入仓库 |
|------|------|-----------|
| `.planning/phases/35-e2e/benchmarks/v3-acceptance-YYYYMMDDTHHMMSSZ.json` | 机器可读历史，schema_version=1 | 是（PR commit） |
| `docs/runbooks/v3-acceptance-report-YYYYMMDD.md` | 人类可读 + 签字 | 是（PR commit） |
| `.planning/phases/35-e2e/benchmarks/bench-*.json` | Plan 01 perf-benchmark 原始输出 | 是（CI artifact + 本地落盘） |

PR description 必须贴 summary 表（PASS / FAIL / SKIP / WARN 数字），格式参 `scripts/v3-acceptance-checklist.sh::write_md_report()` 的「## 汇总」段。

---

## 6. 回归触发（Rollback Trigger）

| 触发条件 | 动作 | 责任 |
|---------|------|------|
| 任一 BASE 持续失败 2 个版本 | 停止 release + 回流对应 phase | 当值发布工程师 |
| 任一 pitfall 场景失败（M5 / M13 / C6） | 触发 `/gsd-plan-phase 35 --gaps` 创建补丁 phase | 验收签字人 |
| 任一 REQ-F* 在签字 PR 中标 FAIL | PR 不合并；REQ-ID 入 deferred-items.md | reviewer |
| CI `perf-benchmark` job 红 | 阻塞 PR；走 `/gsd-debug` 流程 | PR author |
| CI `image-size-regression` job 红 | 阻塞 PR；走 image bloat 排查（参 `v3-upgrade-guide.md` §镜像体积） | PR author |

---

## 7. 快速诊断命令

```bash
# 1. 干跑确认枚举完整（CI / 本机均可，无副作用）
bash scripts/v3-acceptance-checklist.sh --track=all --dry-run \
  | grep -cE '───── (REQ-F[1-8]-[A-E]|BASE-0[1-4]|M5|M13|C6) '   # 期望 ≥ 34

# 2. 只跑 BASE 性能基线
bash scripts/v3-acceptance-checklist.sh --track=base --env=auto

# 3. 只跑 doctor 维度（容器内已起 cloud-claude）
bash scripts/v3-acceptance-checklist.sh --track=req-f6 --target-container=<ctr>

# 4. 看最近一次报告 head
ls -t docs/runbooks/v3-acceptance-report-*.md | head -1 | xargs head -40

# 5. 用 jq 抽 JSON summary
jq '.summary' .planning/phases/35-e2e/benchmarks/v3-acceptance-*.json | tail -10
```

---

## 8. 参考

- 主脚本：`scripts/v3-acceptance-checklist.sh`
- Plan 01 性能基线：`scripts/perf-benchmark.sh` / `scripts/cold-start-benchmark.sh`
- Plan 02 UAT：`scripts/uat-network-resilience.sh` / `scripts/degradation-regression.sh`
- Plan 03 运维手册：`docs/runbooks/v3-upgrade-guide.md` / `v3-apparmor-deployment.md` / `v3-doctor-troubleshoot.md` / `v3-persistent-volume-lifecycle.md` / `v3-error-code-index.md`
- Plan 04 CI gates：`.github/workflows/ci.yml`（jobs `perf-benchmark` + `image-size-regression`）
- 既有可复用脚本：`scripts/verify-fuse-compat.sh` / `scripts/ci-doctor-grep.sh` / `scripts/verify-managed-image.sh` / `deploy/scripts/host-preflight.sh`
- REQ-ID 全表：`.planning/REQUIREMENTS.md` Traceability Matrix
- Phase 35 上下文：`.planning/phases/35-e2e/35-CONTEXT.md`
