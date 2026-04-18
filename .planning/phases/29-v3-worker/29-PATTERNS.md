# Phase 29 PATTERNS — 代码模式映射

> 每个将要新增/修改的文件，找到最接近的现有 analog 与可复用 pattern，供 `gsd-planner` 在拆 plan 时直接引用。
>
> **编排备忘：** 本文件不复述 CONTEXT.md 的决策文字；只给"哪里抄、怎么抄、差异在哪"。

---

## 汇总表（按 Sub-scope A..F 对齐 29-RESEARCH.md §Sub-scope 映射）

| Sub-scope | 新/改文件 | 最接近 Analog（文件 + 行号） | 复用 Pattern | 与 Analog 的差异 | Planner 消费指引 |
|-----------|-----------|------------------------------|--------------|------------------|------------------|
| **A** | `deploy/docker/managed-user/Dockerfile` | 同文件 `9-41` 现 apt 安装段；`48-54` KasmVNC `.deb` 下载 + `dpkg -i` 段；`71-76` userdel + useradd 段；`85-95` COPY + chmod 段；`99-102` EXPOSE + ENTRYPOINT 尾段 | **apt 合并 RUN**（`apt-get update && apt-get install -y --no-install-recommends ... && rm -rf /var/lib/apt/lists/*`，最后一行清理，单 RUN 一层）；**`ARG <NAME>_VERSION` + 下载 .deb + `dpkg -i` + `rm` + `rm -rf /var/lib/apt/lists/*`**（KasmVNC 段是最贴近的 mergerfs `.deb` 模板）；**COPY + chmod +x 单 RUN 合并**（95 行）；**ENTRYPOINT 用 exec form JSON 数组**（102 行） | A1：新增 BuildKit cache mount `--mount=type=cache,target=/var/cache/apt,sharing=locked`（现 RUN 无 cache mount）；A2：现 `rm -rf /var/lib/apt/lists/*` 要保留，但需配合 `rm -f /etc/apt/apt.conf.d/docker-clean` 否则 cache mount 被自动清理；A3：新增 `tini` 安装 + 修改 ENTRYPOINT 为 `["/usr/bin/tini","--","/usr/local/bin/entrypoint.sh"]`；A4：新增 `/etc/cloud-claude/*.version` 写入；A5：新增预建目录 + chown 1000:1000（现 85-87 行只 chown `/workspace`，需扩展） | **Plan-A 镜像构建基线** 引用 KasmVNC `.deb` pattern 作为 mergerfs 下载模板；所有新增 `RUN` 必须以"单 RUN + `--no-install-recommends` + `rm -rf /var/lib/apt/lists/*`"收尾；ENTRYPOINT 行替换（不新增），exec form 保持；apt 清单合并到现 `9-41` 段时按字母序插入 `tini`，`mergerfs`/`mutagen` 走 RUN curl 而不是 apt |
| **B** | `Dockerfile`（新增 RUN 段）+ `/etc/cloud-claude/mergerfs.version` + `/etc/cloud-claude/mutagen.version` | 同 Dockerfile `48-54` KasmVNC `.deb` 下载 pattern；`66-69` locale-gen + ENV 段是"构建期写配置文件"最简 pattern | `ARG <COMPONENT>_VERSION=X.Y.Z` + `RUN ARCH=$(dpkg --print-architecture) && curl -fsSL -o /tmp/xxx ... && sha256sum ... && dpkg -i ... && rm /tmp/xxx`；`echo "v0.18.1" > /etc/cloud-claude/mutagen.version` 风格（echo + 单行写文件） | B1：KasmVNC 没做 checksum 校验，mergerfs/mutagen **必须** `sha256sum -c -`（RESEARCH §Standard Stack 给出两个 sha256）；B2：mutagen 是 tarball（`.tar.gz`）不是 `.deb`，解压到 `/opt/mutagen-agents.tar.gz`（保留原 tarball，不解包子 tarball——Phase 31 runtime extract） | **Plan-B 二进制预置** 单独成 plan；Dockerfile 新 RUN 紧跟现 KasmVNC 段之后；每个下载必须有 `sha256sum -c -` 行，失败即 build fail；版本元数据文件在同 RUN 内写入（一次 layer） |
| **C** | `deploy/docker/managed-user/entrypoint.sh` 改造 + 新增 `deploy/docker/managed-user/tmux.conf` + 新增 `deploy/docker/managed-user/profile.d-cloud-claude.sh` | entrypoint.sh 现有函数风格 `12-43 write_desktop_config` / `45-58 wait_for_x_display`；`2` `set -euo pipefail` 骨架；`60-94` 的"SSH setup / user rename / chown / chpasswd" 串行初始化段；`95-101` sysctl + FUSE 准备；`186` 尾行 `exec /usr/sbin/sshd -D -e` | **函数命名 snake_case + 动词前缀**（`write_desktop_config` / `wait_for_x_display`）——新函数沿用 `prepare_*` / `assert_*` 前缀；**内联 heredoc 写 yaml/conf**（`105-142` kasmvnc.yaml heredoc 是最贴近的"构建期 COPY + runtime heredoc 兜底"pattern，但 v3 的 tmux.conf 应走 **COPY 静态文件**路径，减少 runtime I/O）；**fail-fast**（现 `set -euo pipefail` 加 `62-64` `ssh-keygen -A` 条件执行，v3 的 `assert_tmux_version` 失败应 `exit 1`）；**尾行 exec 原地替换**（现 186 行 `exec /usr/sbin/sshd -D -e`） | C1：v3 阶段插入位置——在 `62-64` ssh-keygen 之后、`186` exec sshd 之前；**严禁挤进 `write_desktop_config` / `wait_for_x_display` 内部**（它们是 KasmVNC 桌面逻辑的私有路径）；C2：`prepare_v3_dirs` 的 chown 只改 `/home/claude` / `/workspace-hot` / `/workspace-cold` / `/var/lib/claude-persist`，**不**重 chown 现有 `/workspace`（那段由 90 行 `chown -R ${RUN_USER}:${RUN_USER} /workspace` 负责，v3 不介入）；C3：tmux.conf 按 **COPY 静态文件**（Dockerfile `COPY deploy/docker/managed-user/tmux.conf /etc/tmux.conf`），不在 entrypoint 内 heredoc 生成；C4：mergerfs 在本阶段**只校验版本**，`prepare_mergerfs` 里是 `mergerfs --version \| grep 2.41.1`，**不调 mount 命令**（Phase 31 消费） | **Plan-C entrypoint & 配置** 成独立 plan；每个新函数先定义（上半部）再在"串行初始化段"尾部调用；按 D-09 顺序 `prepare_fuse` → `prepare_v3_dirs` → `prepare_mutagen_agent` → `prepare_mergerfs` → `assert_tmux_version`；sshd_config 修改见同一 plan（单文件追加 4 行），tmux.conf/profile.d 新建文件各走 Dockerfile COPY |
| **C** (子项) | `deploy/docker/managed-user/sshd_config` 追加 | 同文件 `1-15`（15 行 Port/Protocol/Auth 键值对） | **扁平 key-value 一行一条**；**无注释分组** | 追加 `ClientAliveInterval 15` / `ClientAliveCountMax 8` / `MaxSessions 30` / `MaxStartups 60:30:120` 四行 | 与 entrypoint 改造放入同一 Plan-C；追加至文件末尾（14 行后），`Subsystem sftp` 之前的扁平风格保持 |
| **D** | `internal/agentapi/contracts.go`（追加 struct + 字段） | 同文件 `13-19` `SSHKeyEntry` struct 定义；`21-41` `HostActionRequest` struct 定义（注意 `SSHKeys []SSHKeyEntry \`json:"ssh_keys,omitempty"\`` 是最直接的 analog） | **struct 首字母大写字段 + 小写 JSON tag snake_case**；**`omitempty` 对 slice 字段**（`SSHKeys` 40 行是先例）；**新 struct 紧贴使用它的 struct 上方定义** | 新增 `VolumeMount` struct（`Name`/`Target`/`ReadOnly`/`Labels` 字段 + JSON tag）；`HostActionRequest` 追加 `Volumes []VolumeMount \`json:"volumes,omitempty"\`` 放在 `SSHKeys` 之后 | **Plan-D agentapi contract 扩展** 紧贴现 `SSHKeyEntry`/`SSHKeys` 的 precedent；字段顺序严格：`Name,Target,ReadOnly,Labels`；`Labels` 用 `map[string]string` 保持与 `HostActionRequest.Labels` 一致的类型；不改动现有字段的顺序和 tag |
| **D** (子项) | `internal/runtime/tasks/worker.go:createHost` args 拼接 | 同文件 `157-170` docker create args 数组；`179-187` env + `-v homeDir:homeMount` bind mount 段；`189-191` Labels 遍历追加 `--label k=v` 的 pattern | **`args := []string{"create", ...}`**；**`args = append(args, "-e", "KEY=VALUE")`**；**`for k,v := range request.Labels { args = append(args, "--label", fmt.Sprintf("%s=%s", k, v)) }`** 遍历模式 | D1：插入点在 `187` `-v homeDir:homeMount` 行之后、`189` Labels 遍历之前；D2：遍历 `request.Volumes` 追加 `--mount type=volume,src=<Name>,dst=<Target>[,readonly]`（注意是 `,readonly` 不是 `,ro`——RESEARCH §8 明确）；D3：**不**对 Volumes 内 Labels 做容器 label 注入（D-19 已禁止，与 `request.Labels` 的遍历是两个独立语义） | 同 Plan-D；必须在 `append(args, request.ImageName)` 之前完成拼接；空 `request.Volumes` slice 等价于 nil，零新增 args（v2.0 JSON round-trip 保持兼容） |
| **D** (子项) | `internal/runtime/tasks/worker_volume_test.go`（新文件） | `internal/runtime/tasks/ssh_inject_test.go` 全文 | **`fakeContainer` + `execInContainer` 注入**（ssh_inject_test.go `18-64` + `101-109`）；**`setupInjectTest` helper + `t.Cleanup(func(){ execInContainer = prev })`**；**`t.Run("case_name", ...)` 子测试分组**；**断言风格 `if got := ...; got != want { t.Fatalf(...) }`** | D4：本阶段的测试**不需要** `execInContainer` 注入（VolumeMount JSON round-trip + docker args 拼接都是纯函数逻辑，无容器 IO）；D5：新建 `worker_volume_test.go` 专注两个测试点：(a) `json.Marshal/Unmarshal` round-trip（含空/nil Volumes）；(b) args 拼接快照（`--mount type=volume,src=X,dst=Y,readonly` 出现位置与空 slice 不新增 args）；D6：若 worker.go 的 `createHost` 整体难以独立测试（涉及 docker pull / network provider），可抽出 `buildCreateArgs(request)` helper 独立测 | 同 Plan-D；测试粒度"纯拼接函数"优先于"整体 createHost"；若必须碰 createHost，复用 ssh_inject_test.go 的 fakeContainer 模式但 **不 fake docker pull**——通过 `buildCreateArgs` 提取重构 |
| **E** | `deploy/scripts/host-preflight.sh`（**已存在**，非新增 — 对 D-23 的路径表述是修正点） | **同文件 `1-45`（已存在的 host-preflight.sh）**！`1-2` shebang + `set -euo pipefail`；`4-9` `require_cmd` helper；`11-14` 必需命令检查；`15-18` nft/iptables 或组检查；`21-27` FUSE kernel module 条件检查；`43-44` `mkdir -p /var/lib/...` 初始化 | **`#!/usr/bin/env bash` + `set -euo pipefail`**；**`require_cmd <name>` helper 模式（以 `>&2` 输出 + `exit 1`）**；**条件检查 → 失败 `>&2` 打印 + 修复提示 + `exit 1`**；**命令可用性 `command -v` 判定** | E1：**D-23 的"新增 `deploy/host-preflight.sh`"与实际代码结构存在路径不一致**——已有脚本位于 `deploy/scripts/host-preflight.sh`。建议在 Plan-E 把 v3.0 检测逻辑**追加到已有文件**（而不是新建文件），并在 DISCUSSION-LOG 或 CONTEXT 附录中补注路径修正；E2：新增 AppArmor override 检测逻辑——`source /etc/os-release`、判断 `${VERSION_ID}` 是否以 `25.04` 开头、判断 `/etc/apparmor.d/local/fusermount3` 是否含 `capability dac_override,`；E3：失败时打印的修复命令需严格遵循 RESEARCH §host-preflight 骨架（`tee -a` 追加 + `apparmor_parser -r /etc/apparmor.d/fusermount3`）；E4：非 Ubuntu 25.04 宿主机 → 打印"无需 override"信息并继续检查其他项（不独立 `exit 0`，融入现脚本的多检查串行流程） | **Plan-E host-preflight 扩展 + 运维文档** 成独立 plan；Plan 第一条任务必须先澄清"追加到 `deploy/scripts/host-preflight.sh`"而非新建 `deploy/host-preflight.sh`（与 CONTEXT D-23 路径表述不一致时以现有文件为准）；第二条任务插入 AppArmor 检测函数（建议命名 `check_apparmor_fusermount3_override`）；第三条任务更新 `deploy/README.md` 或 `docs/` 新增"v3.0 AppArmor override 部署"一节 |
| **F** | `deploy/docker/managed-user/image.lock` 追加 | 同文件 `1-9`（9 行 YAML 键值对） | **扁平 YAML key-value**；**无嵌套 map / list**；**`true/false`/`never-implicit-latest`/`wipe-/workspace` 等字符串直接书写，不加引号（除非含特殊字符）**；**字段顺序：标识 → 版本 → 路径 → 策略** | 在 `factory_reset_mode: wipe-/workspace` 之后追加 6 字段（D-26 列表）；`tmux_version_min: "3.4"` **必须加引号**（避免 YAML 解析成 float 3.4） | **Plan-F image.lock 扩展 + CI gate** 同 plan；image.lock 改动是一次 YAML 尾部追加，无重排序；`tmux_version_min: "3.4"` 引号必须保留（Go YAML/awk 解析双重友好） |
| **F** (子项) | `.github/workflows/build-images.yml`（或新 workflow） | 同文件 `73-87` `Build and push` step；`39-42` checkout step；`56-57` env 预设 step（`echo "X=Y" >> $GITHUB_ENV`） | **`- name:` + `shell: bash` + `run: |`** 内联 bash；**checkout → buildx setup → build & push** 已经是 matrix 结构；**可以在 matrix 中的 managed-user 那一行添加 post-build step** | F1：现 `build-images.yml` 是**多 image matrix**，managed-user 只是其中一行（32-34）；size gate 必须只对 `matrix.image == 'managed-user'` 生效 — 用 `if:` 表达式；F2：build-push-action 直接 push 到 registry，镜像不留本地，**必须**配 `load: true` 或在 gate step 里 `docker pull ${{ steps.meta.outputs.tags }}` 一个 tag 回来后再 inspect（或用 `outputs: type=docker,dest=...` + `docker load`）；F3：现 workflow 用 buildx 多平台 `linux/amd64,linux/arm64`，amd64 一路足够做 size gate（arm64 本阶段 defer） | **Plan-F CI gate** 子任务：(1) 修改 `build-images.yml` managed-user 路径改 `load: true`（仅 amd64，matrix 内 `if: matrix.image == 'managed-user'`）或拆分出 amd64-only 前置 build；(2) 新 step `Assert managed-user image size ≤ 700MB (BASE-04)`，按 RESEARCH §CI gate bash 片段；(3) 失败 `::error::` 前缀 + `docker history` 自动输出；(4) 不 touch build-managed-image.sh（D-30）；如 matrix 改造复杂度失控，可拆出独立 `image-size-gate.yml` workflow 单独编排 managed-user 的 build + size check（需与用户再确认） |

---

## Reusable Assets（可直接沿用的既有资产）

### Shell

| # | 资产 | 位置 | 复用方式 |
|---|------|------|----------|
| S1 | `set -euo pipefail` + `#!/usr/bin/env bash` 骨架 | 所有 `deploy/**/*.sh` 第 1-2 行 | 所有新脚本首行必须同款 |
| S2 | `require_cmd` helper | `deploy/scripts/host-preflight.sh:4-9` | Plan-E 的 AppArmor 检测直接调用；不重复实现 |
| S3 | entrypoint.sh 的 "串行初始化 + 尾行 exec 守护进程" 编排模式 | `deploy/docker/managed-user/entrypoint.sh:60-186` | Plan-C 的 v3 阶段插入复用此模式，不引入并行 |
| S4 | `wait_for_x_display` 的"轮询 + 超时 + 返回码"模式 | `deploy/docker/managed-user/entrypoint.sh:45-58` | 若 `prepare_mutagen_agent` 需要等待某 tarball extract 完成（本阶段不需要），按此模式实现 |
| S5 | heredoc 写配置的 pattern | `deploy/docker/managed-user/entrypoint.sh:105-142` (kasmvnc.yaml) | 仅作最终 fallback；**v3 的 tmux.conf 优先走 Dockerfile COPY** |

### Dockerfile

| # | 资产 | 位置 | 复用方式 |
|---|------|------|----------|
| D1 | `ARG <COMPONENT>_VERSION=X.Y.Z` + `curl -fsSL -o /tmp/... && dpkg -i ...` 下载安装 pattern | `deploy/docker/managed-user/Dockerfile:47-54` (KasmVNC) | Plan-B mergerfs `.deb` 直接套用；加 `sha256sum -c -` 校验 |
| D2 | `apt-get update && apt-get install -y --no-install-recommends ... && rm -rf /var/lib/apt/lists/*` 单 RUN 清单 | Dockerfile `9-41` | Plan-A 追加 `tini` 至此清单；新增的 apt 安装**不**另起 RUN |
| D3 | `locale-gen` + ENV 设置序列 | Dockerfile `66-69` | 构建期设置环境变量的直接 precedent |
| D4 | `COPY deploy/docker/managed-user/XXX /usr/local/bin/xxx` + `RUN chmod +x` 模式 | Dockerfile `89-95` | Plan-C 的 tmux.conf 走同一模式（target 路径为 `/etc/tmux.conf`，chmod 644） |
| D5 | `ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]` exec form | Dockerfile `102` | Plan-A 原地修改为 `ENTRYPOINT ["/usr/bin/tini","--","/usr/local/bin/entrypoint.sh"]` |

### Go

| # | 资产 | 位置 | 复用方式 |
|---|------|------|----------|
| G1 | `struct + JSON tag + omitempty` slice 字段 | `internal/agentapi/contracts.go:21-41` (`SSHKeys []SSHKeyEntry`) | Plan-D 的 `Volumes []VolumeMount` 逐字复用该 tag 语法 |
| G2 | docker create args 数组 + append + 遍历 Labels 的拼接模式 | `internal/runtime/tasks/worker.go:157-193` | Plan-D 在 `187` 行后、`189` 行前插入 Volumes 遍历；不重构整段 |
| G3 | `execInContainer` package-level var 注入 fake | `internal/runtime/tasks/worker.go:650-656` + `ssh_inject_test.go:101-109` | Plan-D 的 worker_volume_test.go 若需 exec 层仿真时复用；优先无依赖直测 |
| G4 | `fakeWorkerRepo` 最小 `WorkerRepo` 实现（空方法） | `internal/runtime/tasks/ssh_inject_test.go:66-86` | 需要断言 `RecordEvent` 的测试继承复用 |
| G5 | `t.Run("case_name", ...)` + `setupInjectTest` helper 模式 | `internal/runtime/tasks/ssh_inject_test.go:90-109, 132-318` | 所有新测试沿用此风格 |
| G6 | `firstNonEmpty` helper | `internal/runtime/tasks/worker.go:811-819` | 若新 Volumes 拼接需要默认值兜底可直接用；本阶段可能用不到 |

### CI

| # | 资产 | 位置 | 复用方式 |
|---|------|------|----------|
| C1 | multi-arch buildx + GHCR push 的 matrix 结构 | `.github/workflows/build-images.yml` 全文 | Plan-F 的 size gate 插入到 managed-user 那一行的 post-build step |
| C2 | `echo "X=Y" >> $GITHUB_ENV` 动态 env 传递 | `.github/workflows/build-images.yml:56-57` | Plan-F 若需要把 image tag 传到下一 step 时复用 |

---

## Anti-patterns（v3 明确不走的现有代码路径）

| # | 现有代码路径 | 为什么不要走 | 应该走哪条 |
|---|--------------|--------------|------------|
| AP1 | `worker.go:186` 的 `-v homeDir:homeMount` bind mount 简短语法 | v2.0 已有 bind mount 保留不动；**Volumes 字段必须用 `--mount type=volume,...` 新语法** | RESEARCH §8：`--mount type=volume,src=<Name>,dst=<Target>[,readonly]`；`,readonly` 不是 `,ro` |
| AP2 | 把 v3 初始化逻辑挤进 `entrypoint.sh:write_desktop_config` 或 `wait_for_x_display` 内部 | 这两个函数承担 KasmVNC 桌面私有编排；v3 的 FUSE/mutagen/mergerfs 与 X11 无关系 | 在 `60-101` 的 SSH/user/FUSE 初始化段之后、`155` 启动 Xvnc 之前**另起函数** |
| AP3 | 把 size gate bash 塞进 `build-managed-image.sh` | D-30 明确禁止（build script 保持单一职责） | 仅在 `.github/workflows/*.yml` CI step 实现 gate |
| AP4 | 在 `prepare_mergerfs` 里执行 `mergerfs <branches> /workspace/.mergerfs` 挂载命令 | D-09 / D-11 / RESEARCH §10：本阶段 entrypoint 只校验版本，mount 由 Phase 31 消费 | `mergerfs --version \| grep -F 2.41.1 \|\| exit 1` 即止 |
| AP5 | 从 Dockerfile `apt install` 安装 mergerfs | M3 / D-04：apt 版本 2.33.5 缺 `func.readdir=cor:N` 参数，C1/M1 防御失效 | 走 GitHub release `.deb` + `dpkg -i` |
| AP6 | 给 `HostActionRequest` 加 `VolumesVersion` 或其他 schema 版本字段 | D-22：`omitempty` + Go encoding/json 默认忽略未知字段已足够向后兼容 | 不加版本字段；依赖 omitempty 实现向后兼容 |
| AP7 | 在 entrypoint 里读取 `CLOUD_CLAUDE_MERGERFS_BRANCHES` env 并做分支组装 | D-12 + RESEARCH §10：本阶段只预留环境变量名，读取逻辑归 Phase 31 cloud-claude | 仅在 RESEARCH/运维文档中登记 env 名称 |
| AP8 | 继续用 `/etc/apparmor.d/local/docker-default` 作为 AppArmor override 路径 | RESEARCH §Conflicts with CONTEXT.md + 用户澄清（2026-04-18）确认修正为 `fusermount3` | `/etc/apparmor.d/local/fusermount3` + `apparmor_parser -r /etc/apparmor.d/fusermount3` |
| AP9 | 新建 `deploy/host-preflight.sh`（D-23 原文所暗示的路径） | 与现有 `deploy/scripts/host-preflight.sh` 冲突；**现文件已存在** | 在现有 `deploy/scripts/host-preflight.sh` 尾部追加 AppArmor override 检测函数；CONTEXT D-23 的"新增"表述应在 Plan-E 首任务里澄清为"扩展"（不改 CONTEXT 文本，在 plan 描述里注明） |
| AP10 | 在 v3 的 `prepare_v3_dirs` 里对 `/workspace` 再做一次 `chown -R` | 现 entrypoint.sh `90` 行已经 `chown -R ${RUN_USER}:${RUN_USER} /workspace`，重复 chown 会引发 C5 风险（Mutagen root 反向清空家族的邻近坑位） | 仅对 `/home/claude` / `/workspace-hot` / `/workspace-cold` / `/var/lib/claude-persist` chown；`/workspace` 不动 |

---

## No-close-analog 条目（planner 需自建 pattern）

| # | 文件 / 片段 | 为什么无 analog | 推荐模板来源 |
|---|-------------|------------------|--------------|
| N1 | `deploy/docker/managed-user/tmux.conf`（新文件） | 项目无既有 tmux 配置文件 | RESEARCH §Code Examples §tmux.conf 骨架；Google Shell Style Guide 对配置文件无意见，遵循 RESEARCH 骨架即可 |
| N2 | `deploy/docker/managed-user/profile.d-cloud-claude.sh`（新文件） | 项目无既有 `/etc/profile.d/*.sh` 投放模式 | 简单 `export KEY=VALUE` 一行一条；RESEARCH §Code Examples 已给骨架 |
| N3 | `docs/` 下的"v3.0 AppArmor override 部署"章节 | `docs/` 目录可能尚未建立（需检查） | 若 `docs/` 不存在，放入 `deploy/README.md`；风格参考 `.planning/research/*.md` 的中英混排 + URL 引用风格 |

---

## Planner 消费 Checklist（给 `gsd-planner` 的最后提醒）

1. 本文件 **不**修改 CONTEXT.md / RESEARCH.md；所有新观察（AP9 路径不一致、N3 docs 目录不确定、F2 build-push `load:true` 需求）都以 plan 内描述 + plan checker 建议的方式处理
2. Sub-scope A..F 映射到 6 份独立 PLAN.md（建议编号 `01-image-base` / `02-binaries` / `03-entrypoint-config` / `04-worker-contract` / `05-host-preflight-docs` / `06-imagelock-ci-gate`）
3. 每份 plan 的验收必须回引 RESEARCH §Validation Architecture 三列表中对应的 SC1..SC7
4. Plan-E 必须在首个任务里显式澄清"追加到 `deploy/scripts/host-preflight.sh`"（避免 executor 按 CONTEXT 原文新建 `deploy/host-preflight.sh`）
5. Plan-F CI gate 必须说明 buildx `push: true` 与 size gate `docker image inspect` 的兼容方案（`load:true` 或独立 workflow）

## PATTERNS MAPPED — 6 个 Sub-scope 的 analog、pattern、差异、Planner 指引全部就位；同时捕获 CONTEXT D-23 路径与现有 `deploy/scripts/host-preflight.sh` 不一致（AP9）与 CI gate 的 buildx push/load 兼容问题（F2），供 planner 在 Plan-E / Plan-F 首任务澄清
