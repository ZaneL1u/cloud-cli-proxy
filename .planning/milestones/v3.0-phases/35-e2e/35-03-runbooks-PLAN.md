---
phase: 35-e2e
plan: 03
type: execute
wave: 1
depends_on: []
autonomous: true
files_modified:
  - docs/runbooks/v3-upgrade-guide.md
  - docs/runbooks/v3-apparmor-deployment.md
  - docs/runbooks/v3-doctor-troubleshoot.md
  - docs/runbooks/v3-persistent-volume-lifecycle.md
  - docs/runbooks/v3-error-code-index.md
requirements_addressed:
  - C6
  - M13
  - M5
threat_model_severity: low
must_haves:
  truths:
    - "docs/runbooks/ 下 5 个新 markdown 文件每个头部含 `适用版本: v3.0` 字样"
    - '每个新手册至少 5 个顶层章节（`^## ` 计数 ≥ 5）且含"快速诊断命令"小节'
    - "v3-error-code-index.md 表格行数 ≥ errcodes 注册表实际 code 数（42 条，Phase 34 SUMMARY 锚定），且列出 8 个域（AUTH/DISK/MOUNT/NET/SESSION/SSH/STATE/SYSTEM）"
    - "v3-apparmor-deployment.md 含 `capability dac_override,` + `apparmor_parser -r /etc/apparmor.d/fusermount3` 两条精确命令"
    - "v3-persistent-volume-lifecycle.md §0 或 §1 含到 v3-claude-state-volumes.md 的 markdown 跳转链接（禁止复制已有内容）"
    - "v3-doctor-troubleshoot.md 5 个子节顺序 = network → auth → ssh → mount → disk（与 doctor.go L83 一致）"
  artifacts:
    - path: "docs/runbooks/v3-upgrade-guide.md"
      provides: "v3.0 升级指南：image.lock 版本锁 + 客户端 install.sh 升级流程 + 回滚"
      min_lines: 120
    - path: "docs/runbooks/v3-apparmor-deployment.md"
      provides: "Ubuntu 25.04 AppArmor local override 部署（C6）+ verify-fuse-compat.sh 验证"
      min_lines: 100
    - path: "docs/runbooks/v3-doctor-troubleshoot.md"
      provides: "5 维度排障手册（network/auth/ssh/mount/disk）+ --fix 幂等说明（M14）"
      min_lines: 150
    - path: "docs/runbooks/v3-persistent-volume-lifecycle.md"
      provides: "hot/cold 卷顶层总览 + 跳转到 v3-claude-state-volumes.md（整合不重复）"
      min_lines: 80
    - path: "docs/runbooks/v3-error-code-index.md"
      provides: "42 条错误码索引（8 域） + 生成命令注释 + severity/message/next_action/extended 字段"
      min_lines: 100
  key_links:
    - from: "docs/runbooks/v3-apparmor-deployment.md"
      to: "deploy/scripts/host-preflight.sh::check_apparmor_fusermount3"
      via: "markdown 引用函数名 + 粘贴 D-23 override 规则（/etc/apparmor.d/local/fusermount3）"
      pattern: "/etc/apparmor.d/local/fusermount3"
    - from: "docs/runbooks/v3-doctor-troubleshoot.md"
      to: "internal/cloudclaude/doctor/doctor.go::RunDoctor"
      via: "5 维度顺序引用 + 18 项 check 列表"
      pattern: "network → auth → ssh → mount → disk"
    - from: "docs/runbooks/v3-error-code-index.md"
      to: "internal/cloudclaude/errcodes/ (8 个域文件 init)"
      via: "表格 Code 列等于 Registry 输出（手册内注明生成方式）"
      pattern: "cloud-claude explain"
    - from: "docs/runbooks/v3-persistent-volume-lifecycle.md"
      to: "docs/runbooks/v3-claude-state-volumes.md"
      via: "markdown 跳转链接 `[v3-claude-state-volumes.md](./v3-claude-state-volumes.md)`"
      pattern: "\\]\\(\\./v3-claude-state-volumes\\.md\\)"
---

<objective>
交付 5 章 v3.0 运维手册（docs/runbooks/v3-*.md），锁定 v3.0 运维侧知识资产，对应 Phase Success Criteria #9。
Purpose: 把 Phase 29-34 散落在 SUMMARY / PLAN / CONTEXT 的运维操作收口到 docs/runbooks/，让值班运维无需阅读 `.planning/` 就能：(1) 升级 v2.0→v3.0 / (2) 在 Ubuntu 25.04 部署 AppArmor override / (3) 用 doctor 5 维度排障 / (4) 管理 hot/cold 持久卷 / (5) 查任意错误码含义。所有手册样式、头部约定与既有 `v3-claude-state-volumes.md` 保持一致。
Output: 5 个 markdown 文件，每份独立可 follow，内容自证（含快速诊断命令小节、关联 REQ-ID、代码引用到函数级）。
</objective>

<execution_context>
@.cursor/get-shit-done/workflows/execute-plan.md
@.cursor/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/35-e2e/35-CONTEXT.md
@.planning/phases/35-e2e/35-RESEARCH.md
@.planning/phases/35-e2e/35-PATTERNS.md

<!-- 手册样板与代码信源 -->
@docs/runbooks/v3-claude-state-volumes.md
@deploy/scripts/host-preflight.sh
@scripts/verify-fuse-compat.sh
@internal/cloudclaude/errcodes/codes.go
@internal/cloudclaude/doctor/doctor.go
@deploy/docker/managed-user/image.lock
@scripts/install.sh
</context>

<interfaces>
<!-- 头部模板（PATTERNS.md Pattern G，引自 v3-claude-state-volumes.md:1-7，5 份新手册必须完全照抄样式） -->

```markdown
# <Title>（v3.0+）

> 适用版本：v3.0 起；对应阶段 Phase <N>（<phase-id>）
> 关联需求：<REQ-ID>（<短述>） / <REQ-ID>（<短述>）

---
```

<!-- 章节骨架（最少 5 个 `## `） -->

```markdown
## 1. 背景
## 2. <核心规范/流程>
## 3. 生命周期 / 操作步骤
## 4. <Audit / 清单 / 具体场景>
## 5. 故障排查
## 6. 快速诊断命令
## 7. 参考
```

<!-- 每章必含的"快速诊断命令"小节（CONTEXT.md L88） -->

```markdown
### 快速诊断命令

```bash
# 3-5 条可 copy-paste 命令
cloud-claude doctor --json | jq '.summary'
docker ps --filter label=com.cloud-cli-proxy.managed=true
```
```

<!-- doctor 5 维度顺序（doctor.go L83-84） -->
network → auth → ssh → mount → disk

<!-- errcodes 8 个域 -->
AUTH / DISK / MOUNT / NET / SESSION / SSH / STATE / SYSTEM

<!-- AppArmor override 规则（host-preflight.sh L51-68，禁止修改字面量） -->
path:  /etc/apparmor.d/local/fusermount3
rule:  capability dac_override,
reload: sudo apparmor_parser -r /etc/apparmor.d/fusermount3
</interfaces>

<tasks>

<task type="execute" id="35-03-T1">
  <name>Task 1: 升级指南 + AppArmor 部署（2 章）</name>
  <files>docs/runbooks/v3-upgrade-guide.md, docs/runbooks/v3-apparmor-deployment.md</files>
  <read_first>
    - docs/runbooks/v3-claude-state-volumes.md（整份 — Pattern G 头部 + 8 章骨架 + "故障排查"三元格式）
    - deploy/scripts/host-preflight.sh（11-73 行 — check_apparmor_fusermount3，**D-23 override 路径与规则字面量必须一字不差引用**）
    - scripts/verify-fuse-compat.sh（42-58 行 AppArmor 检测 + 整个阶段 1-4 结构）
    - deploy/docker/managed-user/image.lock（local_dev_image_name + sha，升级指南头部引用）
    - scripts/install.sh（客户端升级流程样板）
    - .planning/phases/35-e2e/35-PATTERNS.md Pattern G + L572-577（引用关系表格）
    - .planning/phases/29/（若存在）的 SUMMARY — 回溯 v2.0→v3.0 具体变动项
  </read_first>
  <action>

### 1. `docs/runbooks/v3-upgrade-guide.md`（≥ 120 行）

章节结构（严格按此顺序，`^## ` 计数 ≥ 7）：

1. **背景** — v2.0 → v3.0 架构差异（三层文件映射 + tmux 包装 + claude_account 持久化 + doctor v3 + 错误码体系）
2. **升级前置条件清单** — 表格：控制面版本 / 镜像版本 / CLI 版本 / 可选 AppArmor override 三档
3. **控制面升级步骤** — 按顺序编号 1-N：
   - `git pull` → `docker compose pull control-plane worker host-agent`
   - `migrate up`（显式列出 Phase 30 migration `0014_claude_account_persistent_volume.sql` 编号）
   - 回滚策略：列出 migration `migrate down` + 旧镜像 pin 方式
4. **镜像升级步骤** — 引用 `deploy/docker/managed-user/image.lock` 字面量路径 + `image_version: v3.0.0` + 触发 worker 重建容器方式
5. **CLI 客户端升级** — `curl -fsSL .../install.sh | bash` 流程 + 升级后 `cloud-claude --version` 应输出 `v3.0.x`
6. **升级后自检**：
   ```bash
   cloud-claude doctor --json | jq '.summary'       # 全部 pass
   cloud-claude explain MOUNT_MUTAGEN_VERSION_SKEW  # v3.0 新错误码可 explain
   docker exec <ctr> mount | grep mergerfs          # func.readdir=cor:4,cache.attr=30,...
   ```
7. **常见回归与回滚触发条件** — 5 条：首连 > 8s / 断网 30s 内 claude 退出 / Mutagen 版本漂移 / mergerfs 参数漂移 / AppArmor 阻断
8. **快速诊断命令** — 3-5 条
9. **参考** — 引用到函数级：`internal/runtime/tasks/worker.go::createHost`、`internal/cloudclaude/doctor/doctor.go::RunDoctor`、`deploy/docker/managed-user/entrypoint.sh::prepare_persistent_state`

### 2. `docs/runbooks/v3-apparmor-deployment.md`（≥ 100 行）

章节结构（关联 PITFALL **C6**）：

1. **背景** — Ubuntu 25.04 AppArmor 默认禁止嵌套 FUSE → sshfs + mutagen-agent + mergerfs 三路并发失败的现象（PITFALLS C6 一字不改引用）
2. **适用范围** — 仅 Ubuntu ≥ 25.04 宿主机（`/etc/os-release` 的 `VERSION_ID` ≥ "25.04"，算法同 host-preflight.sh L29-35）
3. **检测**：
   ```bash
   bash deploy/scripts/host-preflight.sh  # 若缺 override 会 advisory 输出修复指令
   bash scripts/verify-fuse-compat.sh     # 三路 FUSE 挂载烟测
   ```
4. **部署步骤**（**D-23 锁定**字面量）：
   - 写 `/etc/apparmor.d/local/fusermount3`，内容**唯一一行** `capability dac_override,`
   - 重载 profile：`sudo apparmor_parser -r /etc/apparmor.d/fusermount3`
   - 验证：`sudo aa-status | grep fusermount3`
5. **失败场景与恢复**：
   - `apparmor_parser` 报错 → 回滚 `rm /etc/apparmor.d/local/fusermount3 && sudo apparmor_parser -r /etc/apparmor.d/fusermount3`
   - docker 容器启动仍报 `Operation not permitted` → `docker info | grep apparmor` 确认 docker 未设置 `--security-opt apparmor=unconfined`
6. **三路并发 FUSE 回归测试** — `verify-fuse-compat.sh` 的阶段 2-4 具体命令 + 期望输出片段
7. **快速诊断命令** — 3-5 条
8. **参考** — `deploy/scripts/host-preflight.sh::check_apparmor_fusermount3` + `scripts/verify-fuse-compat.sh` + RESEARCH `.planning/research/PITFALLS.md` C6

### 共用格式约束（两文件都要）：
- 头部严格按 Pattern G 模板（`适用版本: v3.0 起` + `关联需求: <REQ-ID>` + `---`）
- 每章末尾（或顶层）必含 `### 快速诊断命令` 小节（CONTEXT L88 要求）
- 代码块语言标注：bash / markdown / json / sql 不省略
- **禁止**使用表情符号、AI 腔（如"值得注意的是"、"总的来说"）、英文占位（只保留代码示例 + 技术术语为英文）
  </action>
  <acceptance_criteria>
    - `test -f docs/runbooks/v3-upgrade-guide.md` 退出码 0
    - `test -f docs/runbooks/v3-apparmor-deployment.md` 退出码 0
    - `wc -l < docs/runbooks/v3-upgrade-guide.md` 输出 ≥ 120
    - `wc -l < docs/runbooks/v3-apparmor-deployment.md` 输出 ≥ 100
    - `grep -c '^## ' docs/runbooks/v3-upgrade-guide.md` 输出 ≥ 7
    - `grep -c '^## ' docs/runbooks/v3-apparmor-deployment.md` 输出 ≥ 5
    - `grep -qF '适用版本：v3.0' docs/runbooks/v3-upgrade-guide.md` 退出码 0
    - `grep -qF '适用版本：v3.0' docs/runbooks/v3-apparmor-deployment.md` 退出码 0
    - `grep -qF '快速诊断命令' docs/runbooks/v3-upgrade-guide.md` 退出码 0
    - `grep -qF '快速诊断命令' docs/runbooks/v3-apparmor-deployment.md` 退出码 0
    - `grep -qF 'capability dac_override,' docs/runbooks/v3-apparmor-deployment.md` 退出码 0（D-23 字面量）
    - `grep -qF 'apparmor_parser -r /etc/apparmor.d/fusermount3' docs/runbooks/v3-apparmor-deployment.md` 退出码 0
    - `grep -qF '/etc/apparmor.d/local/fusermount3' docs/runbooks/v3-apparmor-deployment.md` 退出码 0
    - `grep -qF 'image.lock' docs/runbooks/v3-upgrade-guide.md` 退出码 0
    - `grep -qF 'v3.0.0' docs/runbooks/v3-upgrade-guide.md` 退出码 0
  </acceptance_criteria>
  <done>升级指南 + AppArmor 手册落地，字面量与 host-preflight.sh / image.lock 完全对齐。</done>
</task>

<task type="execute" id="35-03-T2">
  <name>Task 2: doctor 排障手册 + 持久卷整合（2 章）</name>
  <files>docs/runbooks/v3-doctor-troubleshoot.md, docs/runbooks/v3-persistent-volume-lifecycle.md</files>
  <read_first>
    - docs/runbooks/v3-claude-state-volumes.md（整份 — 整合规则源文件，**禁止复制其任何章节**）
    - internal/cloudclaude/doctor/doctor.go（1-120 行 — 5 维度顺序、Status 字面量 pass/warn/fail/skip、Report schema_version=1）
    - internal/cloudclaude/doctor/ 目录 ls — 各维度 check 文件存在性
    - scripts/ci-doctor-grep.sh（整份 — M14 断言三要素：schema_version / next_action / 错误码 regex，作为手册 §"CI gate"小节的引用）
    - .planning/phases/35-e2e/35-PATTERNS.md Pattern I + J（doctor 结构 + persistent-volume 整合规则）
    - .planning/phases/35-e2e/35-CONTEXT.md §"运维手册与验收清单形式"
  </read_first>
  <action>

### 1. `docs/runbooks/v3-doctor-troubleshoot.md`（≥ 150 行）

**关联需求：** REQ-F6-A / REQ-F6-B / REQ-F6-C / REQ-F6-D / M13 / M14 全部引用在头部。

章节结构：

1. **背景** — doctor v3 从"PASS/FAIL 黑盒"升级为"5 维度 + 四要素 + --fix"（Phase 34 交付）
2. **输出格式契约** —
   - 四要素：`[✓]/[!]/[✗]` + 中文原因 + `建议: <next_action>` + `错误码: <CODE>`
   - JSON：`schema_version=1` 锁死；`status ∈ {pass,warn,fail,skip}`；示例 JSON 片段贴 10 行左右
   - 退出码：`0`=全 pass、`1`=至少一 warn、`2`=至少一 fail（对齐 brew doctor，REQ-F6-D）
3. **五维度检查逻辑**（顺序必须 network → auth → ssh → mount → disk，Pattern I）：
   - **3.1 network**（3 项）：dns_resolve / gateway_reachable / egress_ip_visible → 常见码 `SYSTEM_DNS_RESOLVE_FAILED` / `NET_EGRESS_IP_DRIFT` → 排障步骤编号列表 → 修复命令
   - **3.2 auth**（3 项）：config_present / entry_token_valid / oauth_valid → 常见码 `AUTH_CONFIG_MISSING` / `AUTH_TOKEN_EXPIRED` / `NET_OAUTH_EXPIRED` / `AUTH_OAUTH_REFRESH_FAILED`
   - **3.3 ssh**（4 项）：ssh_config / keepalive_sane / known_hosts_clean / sshd_baseline → 常见码 `SESSION_KEEPALIVE_TOO_AGGRESSIVE` / `SSH_KNOWN_HOSTS_CONFLICT` / `SSH_SSHD_KEEPALIVE_DRIFT`
   - **3.4 mount**（5 项）：mutagen_version / mergerfs_opts / sshfs_alive / hot_size / ssh_ready → 常见码 `MOUNT_MUTAGEN_VERSION_SKEW` / `MOUNT_MERGERFS_FAILED` / `MOUNT_SSHFS_DISCONNECTED` / `MOUNT_MUTAGEN_WHITELIST_REJECT`
   - **3.5 disk**（3 项）：local_free / container_free / mutagen_data_size → 常见码 `DISK_LOCAL_LOW` / `DISK_CONTAINER_LOW` / `DISK_MUTAGEN_DATA_BLOAT`
   - 每个维度一个子节，每个子节包含：检查项列表 / 常见错误码 / 排障 3-5 步 / 修复命令 copy-paste
4. **`--fix` 自动修复能力**（REQ-F6-C：至少 5 类）— 表格列出每类修复的幂等性与回退：
   - mutagen agent 重启 / FUSE 残留挂载清理 / known_hosts 冲突 / OAuth token 刷新 / DNS 缓存 flush
5. **降级历史第一屏（M13 锁定）** — 说明 doctor 启动时如何展示 last-session.json 的 downgrade_chain；给 `--json` 片段示例
6. **CI gate 集成（M14）** — 引用 `scripts/ci-doctor-grep.sh` 三断言（schema_version / next_action / 错误码 regex）
7. **故障排查案例集**（≥ 3 例，借鉴 v3-claude-state-volumes.md §6 三元格式：事件 → 排查步骤 → 修复命令）：
   - 案例 1：首连后 `[✗] mount.mergerfs_opts` → `MOUNT_MERGERFS_FAILED` 完整排障
   - 案例 2：断网恢复后 `[!] auth.oauth_valid` → `NET_OAUTH_EXPIRED` → 先 `claude login` 再重试
   - 案例 3：CI 跑 `--json` 但 jq 拿到空对象 → 退出码 2 但没 stderr，排查 `--verbose`
8. **快速诊断命令** — 3-5 条
9. **参考** — `internal/cloudclaude/doctor/doctor.go::RunDoctor` + 各维度 `.go` 文件（引到函数级）

### 2. `docs/runbooks/v3-persistent-volume-lifecycle.md`（≥ 80 行）

**整合规则（PATTERNS Pattern J 锁定）：禁止复制 `v3-claude-state-volumes.md` 任何章节内容。**

章节结构：

1. **背景** — v3.0 持久化矩阵：
   - `claude-state-<account_id>` volume → 已在 `v3-claude-state-volumes.md` 详述，**仅 link 跳转**
   - hot / cold mutagen 相关 volume → 本手册主战场
   - mergerfs union layer → 本手册主战场
2. **本手册导航**（PATTERNS Pattern J 样板）：
   ```markdown
   | 问题 | 跳转 |
   |------|------|
   | Claude OAuth 缓存 / ~/.claude 持久化 | → [v3-claude-state-volumes.md](./v3-claude-state-volumes.md) |
   | 同步 Volume 寿命 / GC / Mutagen 数据卷 | § 本文件 §3 |
   | mergerfs union 上层 cold/hot 卷 | § 本文件 §4 |
   ```
3. **Mutagen 数据卷生命周期**（Phase 31-32 范畴）：
   - 容器内路径 `/var/lib/mutagen/`（是否 volume / tmpfs 的判定 — 引用 worker.go）
   - 容器重建后：**不持久化**（每次重建首轮全量同步约 200MB）— 设计决策与权衡
   - 磁盘膨胀诊断：`du -sh /var/lib/mutagen/` → 超过阈值 → `cloud-claude doctor` 输出 `DISK_MUTAGEN_DATA_BLOAT`
4. **mergerfs union layer 上下层关系** —
   - 上层（hot，mutagen 同步目录） / 下层（cold，sshfs mount）
   - `getfattr -n user.mergerfs.branches /workspace/.mergerfs` 期望输出 + 读法
   - `category.create=ff` 的意思 + 新文件总是写 hot 分支
5. **故障排查** — 3 例：
   - mutagen 数据卷撑满 → 重启容器 / tmpfs size 调整
   - mergerfs branches 参数漂移 → 重启容器触发 entrypoint 重建
   - 用户在 hot 写入超过 50MB 被拒 → 引用 `MOUNT_MUTAGEN_WHITELIST_REJECT` + REQ-F1-D 决策
6. **快速诊断命令** — 3-5 条
7. **Deferred（v3.1 backlog）** — 数据卷备份 / hot 容量可调 / 独立 GC 定时任务
8. **参考** — `internal/runtime/tasks/worker.go` + `deploy/docker/managed-user/entrypoint.sh`

### 共用约束：
- 头部严格 Pattern G 模板
- 每章（或全文顶部）含 `### 快速诊断命令` 小节
- 代码引用到函数/行级（Pattern G L375-380 样板）
  </action>
  <acceptance_criteria>
    - `test -f docs/runbooks/v3-doctor-troubleshoot.md` 退出码 0
    - `test -f docs/runbooks/v3-persistent-volume-lifecycle.md` 退出码 0
    - `wc -l < docs/runbooks/v3-doctor-troubleshoot.md` 输出 ≥ 150
    - `wc -l < docs/runbooks/v3-persistent-volume-lifecycle.md` 输出 ≥ 80
    - `grep -c '^## ' docs/runbooks/v3-doctor-troubleshoot.md` 输出 ≥ 7
    - `grep -c '^### 3\.' docs/runbooks/v3-doctor-troubleshoot.md` 输出 = 5（5 维度子节）
    - `grep -qE 'network.*→.*auth.*→.*ssh.*→.*mount.*→.*disk' docs/runbooks/v3-doctor-troubleshoot.md` 退出码 0（顺序断言，容忍 emdash 或箭头）
    - `grep -qF '适用版本：v3.0' docs/runbooks/v3-doctor-troubleshoot.md` 退出码 0
    - `grep -qF '适用版本：v3.0' docs/runbooks/v3-persistent-volume-lifecycle.md` 退出码 0
    - `grep -qF '快速诊断命令' docs/runbooks/v3-doctor-troubleshoot.md` 退出码 0
    - `grep -qF '快速诊断命令' docs/runbooks/v3-persistent-volume-lifecycle.md` 退出码 0
    - `grep -qF 'schema_version' docs/runbooks/v3-doctor-troubleshoot.md` 退出码 0
    - `grep -qE 'MOUNT_MERGERFS_FAILED|MOUNT_MUTAGEN_VERSION_SKEW' docs/runbooks/v3-doctor-troubleshoot.md` 退出码 0
    - `grep -qE '\]\(\./v3-claude-state-volumes\.md\)' docs/runbooks/v3-persistent-volume-lifecycle.md` 退出码 0（跳转链接存在）
    - `! grep -qF '## 4. Audit 事件清单' docs/runbooks/v3-persistent-volume-lifecycle.md`（禁止复制 claude-state-volumes §4，反向断言）
  </acceptance_criteria>
  <done>doctor 排障 + 持久卷两手册落地，前者 5 维度顺序与代码一致，后者仅 link 不复制。</done>
</task>

<task type="execute" id="35-03-T3">
  <name>Task 3: 错误码索引 v3-error-code-index.md（生成 + 自动化）</name>
  <files>docs/runbooks/v3-error-code-index.md</files>
  <read_first>
    - internal/cloudclaude/errcodes/codes.go（整份 — Registry / Format / 42 条 Code 常量）
    - internal/cloudclaude/errcodes/{auth,disk,mount,net,session,ssh,state,system}.go（ls 目录，各域 init MustRegister 列表）
    - internal/cloudclaude/errcodes/explanations.go（ExplainExempt + Extended 列表 — 区分"已有长说明"与"仅表格项"）
    - docs/runbooks/v3-claude-state-volumes.md（Pattern G 头部样式）
    - .planning/phases/35-e2e/35-PATTERNS.md Pattern H（错误码索引专用结构）
    - 若存在：`cmd/errcodes-dump/` 或 `cloud-claude explain --all`（确认生成命令的实际可用形式；若不存在，手册内注明"当前生成方式：遍历代码"）
  </read_first>
  <action>
创建 `docs/runbooks/v3-error-code-index.md`（≥ 100 行）。

章节结构（Pattern H 锁定）：

1. **背景** — v3.0 错误码体系：`<DOMAIN>_<KIND>_<NUM>` 格式（正则 `^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$`，codes.go L56）；8 个域前缀；每条含 Code/Severity/Message/NextAction/ExtendedExplanation 五元
2. **生成方式**（Pattern H 末段样板）：
   ```markdown
   > 本表 Code/Severity/Message/NextAction 与
   > `internal/cloudclaude/errcodes/{auth,disk,mount,net,session,ssh,state,system}.go` 中的
   > `init() MustRegister(...)` 保持一一对应；Extended 列对应 `explanations.go` 的
   > ExtendedExplanations 登记项。
   >
   > 权威来源：`cloud-claude explain <code>` 子命令；若本表与 `explain` 输出不一致，以代码为准。
   ```
3. **按域分组的 42 条表格**（Pattern H 信源，遍历 codes.go L120-178 + 各域 init）：

   每个子节（8 个）：
   ```markdown
   ### 3.1 AUTH_*（CLI 配置 / Entry token / OAuth 刷新）

   | Code | Severity | Message（摘要） | NextAction（摘要） | Extended |
   |------|----------|-----------------|--------------------|----------|
   | AUTH_CONFIG_MISSING | ERROR | ... | ... | ✅ / — |
   | ...  | ... | ... | ... | ... |
   ```

   8 个子节（按 Pattern H 表格顺序）：
   - 3.1 AUTH_* — 4 条
   - 3.2 DISK_* — 3 条
   - 3.3 MOUNT_* — 13 条（含 Phase 31-34 全部）
   - 3.4 NET_* — 6 条（`NET_OAUTH_EXPIRED/EXPIRING_SOON/NOT_FOUND` + `NET_RECONNECT_*` + `NET_TCP_KEEPALIVE_UNSUPPORTED` + `NET_EGRESS_IP_DRIFT`）
   - 3.5 SESSION_* — 7 条
   - 3.6 SSH_* — 2 条
   - 3.7 STATE_* — 3 条（含 `STATE_VOLUME_IN_USE_001`）
   - 3.8 SYSTEM_* — 4 条

   **数量下限（硬约束）**：总条目数必须 ≥ 42（Phase 34 交付数量），可多不可少。实际数以 `grep -hE 'Code = "[A-Z]+_[A-Z]+' internal/cloudclaude/errcodes/*.go | wc -l` 为准。

4. **错误码使用 FAQ**：
   - 如何 grep 所有出现点 — `rg 'errcodes\.(MOUNT_|AUTH_|...)' internal/`
   - `cloud-claude explain <code>` 长说明在哪 — `explanations.go`
   - 新增错误码的 PR checklist — 引用 `codes.go::MustRegister` + codes_test.go
5. **已知跨 Phase 引用场景**（帮助运维把错误码定位到代码）：
   - `STATE_VOLUME_IN_USE_001` → 来源 Phase 33 admin DELETE handler，详见 `v3-claude-state-volumes.md` §3.3
   - `MOUNT_AUTO_DOWNGRADED` → Info 级，Phase 31 三层降级状态机
   - `NET_EGRESS_IP_DRIFT` → 出网 IP 漂移，Phase 29 隧道强制层
6. **快速诊断命令** — 3-5 条（含 `cloud-claude explain <code>`）
7. **参考** — `internal/cloudclaude/errcodes/codes.go::Registry` + `::Format` + `explanations.go::ExplainExempt`

**字面量锁定**：表格 Severity 列值必须 ∈ {`INFO`,`WARN`,`ERROR`,`FATAL`}（codes.go L27-40 枚举）。

**生成校准步骤**（executor 必须执行）：
```bash
# 校准：列出代码中实际 Code，数量 ≥ 42
grep -hE 'Code\s*=\s*"[A-Z]+_[A-Z]+' internal/cloudclaude/errcodes/*.go \
  | awk -F'"' '{print $2}' | sort -u | wc -l
```
  </action>
  <acceptance_criteria>
    - `test -f docs/runbooks/v3-error-code-index.md` 退出码 0
    - `wc -l < docs/runbooks/v3-error-code-index.md` 输出 ≥ 100
    - `grep -c '^## ' docs/runbooks/v3-error-code-index.md` 输出 ≥ 5
    - `grep -c '^### 3\.' docs/runbooks/v3-error-code-index.md` 输出 = 8（8 个域子节）
    - `grep -qE '\\^\\[A-Z\\]\\+_\\[A-Z\\]\\+_\\[A-Z0-9\\]\\+|DOMAIN_KIND_NUM' docs/runbooks/v3-error-code-index.md` 退出码 0（命名规范注明）
    - `grep -qF 'AUTH_' docs/runbooks/v3-error-code-index.md` 且 `grep -qF 'DISK_' ...` 且 `grep -qF 'MOUNT_' ...` 且 `grep -qF 'NET_' ...` 且 `grep -qF 'SESSION_' ...` 且 `grep -qF 'SSH_' ...` 且 `grep -qF 'STATE_' ...` 且 `grep -qF 'SYSTEM_' ...` — 8 域全覆盖
    - `grep -cE '\| [A-Z]+_[A-Z]+_[A-Z0-9_]+ +\|' docs/runbooks/v3-error-code-index.md` 输出 ≥ 42（至少 42 条表行）
    - 反向一致性：`comm -23 <(grep -hE 'Code\s*=\s*"[A-Z]+_[A-Z]+' internal/cloudclaude/errcodes/*.go | awk -F'"' '{print $2}' | sort -u) <(grep -oE '[A-Z]+_[A-Z]+_[A-Z0-9_]+' docs/runbooks/v3-error-code-index.md | grep -E '^(AUTH|DISK|MOUNT|NET|SESSION|SSH|STATE|SYSTEM)_' | sort -u)` 输出为空（注册表中每条 Code 都在手册中出现）
    - `grep -qF '快速诊断命令' docs/runbooks/v3-error-code-index.md` 退出码 0
    - `grep -qF '适用版本：v3.0' docs/runbooks/v3-error-code-index.md` 退出码 0
    - `grep -qF 'cloud-claude explain' docs/runbooks/v3-error-code-index.md` 退出码 0
    - 严重度枚举字面量：`grep -cE '\| (INFO|WARN|ERROR|FATAL) +\|' docs/runbooks/v3-error-code-index.md` 输出 ≥ 42
  </acceptance_criteria>
  <done>错误码索引完整落盘，与 errcodes/*.go 注册表一一对应，反向 diff 为空。</done>
</task>

</tasks>

<verification>
```bash
# 5 个文件存在 + 体量
for f in v3-upgrade-guide v3-apparmor-deployment v3-doctor-troubleshoot v3-persistent-volume-lifecycle v3-error-code-index; do
  test -f "docs/runbooks/${f}.md" || { echo "MISSING: $f"; exit 1; }
  lines=$(wc -l < "docs/runbooks/${f}.md")
  echo "$f : $lines 行"
done

# 头部模板一致
for f in docs/runbooks/v3-upgrade-guide.md docs/runbooks/v3-apparmor-deployment.md \
         docs/runbooks/v3-doctor-troubleshoot.md docs/runbooks/v3-persistent-volume-lifecycle.md \
         docs/runbooks/v3-error-code-index.md; do
  grep -qF '适用版本：v3.0' "$f" || { echo "$f: 头部缺 适用版本"; exit 1; }
  grep -qF '快速诊断命令' "$f" || { echo "$f: 缺快速诊断命令小节"; exit 1; }
done

# 错误码索引与注册表交叉一致
comm -23 \
  <(grep -hE 'Code\s*=\s*"[A-Z]+_[A-Z]+' internal/cloudclaude/errcodes/*.go | awk -F'"' '{print $2}' | sort -u) \
  <(grep -oE '[A-Z]+_[A-Z]+_[A-Z0-9_]+' docs/runbooks/v3-error-code-index.md | grep -E '^(AUTH|DISK|MOUNT|NET|SESSION|SSH|STATE|SYSTEM)_' | sort -u) \
  | tee /tmp/missing-codes.txt
test ! -s /tmp/missing-codes.txt && echo "错误码索引无漏项"
```
</verification>

<success_criteria>
- Phase SC #9：运维手册新增 5 章，每章独立可 follow，字面量与 Phase 29-34 交付一致
- C6（AppArmor）：v3-apparmor-deployment.md 落地部署步骤
- M13（降级可见）：v3-doctor-troubleshoot.md §"降级历史第一屏"明确说明展示逻辑
- M5（APFS case）：虽然本 plan 不覆盖（主要在 Plan 05 真机验收），但 v3-error-code-index.md 列出 `MOUNT_APFS_CASE_INSENSITIVE` 条目
- 每手册头部 `适用版本：v3.0` + `关联需求: ...` 齐全
- v3-error-code-index.md 与 errcodes/*.go 反向 diff 为空
</success_criteria>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| 文档 → 读者 | 手册仅提供 copy-paste 命令；不触发任何 runtime 行为 |
| 文档 → 代码 | 引用代码路径到函数级；如果代码被重命名/删除，手册应在下一次 Phase 更新时同步 |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-35-03-01 | Information Disclosure | 手册示例可能粘贴 token / 密码 | mitigate | 所有 token / password 示例均用 `<BEARER-TOKEN>` / `<ADMIN-JWT>` 占位；sql 示例 `<uuid>` 占位；不留真实 UUID |
| T-35-03-02 | Tampering | 手册命令被运维直接执行 → 误操作 | mitigate | "删除 / force" 类命令前置 ⚠ 警告框；apparmor_parser 重载给出回滚命令；volume rm -f 给出事先检查步骤 |
| T-35-03-03 | Repudiation | 手册出错无追责 | accept | 手册头部 `适用版本: v3.0.x` + git 提交日志作为溯源 |
| T-35-03-04 | Tampering | error-code-index 与 errcodes/*.go 不同步 → 运维依据错误信息操作 | mitigate | 手册开篇声明"权威来源：cloud-claude explain"；CI acceptance criteria 做 `comm -23` 反向 diff 断言 |
</threat_model>

<rollback>
- 5 个 markdown 文件为新建，回滚 = `git rm docs/runbooks/v3-upgrade-guide.md docs/runbooks/v3-apparmor-deployment.md docs/runbooks/v3-doctor-troubleshoot.md docs/runbooks/v3-persistent-volume-lifecycle.md docs/runbooks/v3-error-code-index.md`
- 无代码变更，无反向迁移
- 已有 `docs/runbooks/v3-claude-state-volumes.md` 未修改
</rollback>

<output>
After completion, create `.planning/phases/35-e2e/35-03-SUMMARY.md` documenting:
- 5 手册最终行数
- 头部字段一致性验证结果
- 错误码索引交叉 diff 结果（注册表 vs 手册）
- 运维手册到代码引用的精确映射表
- Deferred 到 v3.1 backlog 的文档项（如数据卷备份）
</output>
