---
phase: quick
plan: 260506-urq
type: execute
wave: 1
depends_on: []
files_modified:
  - deploy/docker/managed-user/Dockerfile
autonomous: true
requirements:
  - FIX-01
must_haves:
  truths:
    - "容器重建后 /usr/local/bin/claude 仍然是可执行的真实 ELF 文件"
    - "install.sh 安装的软链被正确解析并复制为真实二进制"
    - "GitHub release fallback 路径不受影响"
  artifacts:
    - path: "deploy/docker/managed-user/Dockerfile"
      provides: "install.sh 路径下使用 cp -fL 复制真实二进制"
      contains: "cp -fL"
  key_links:
    - from: "Dockerfile:148-156"
      to: "/usr/local/bin/claude"
      via: "cp -fL 跟随符号链接复制真实二进制"
      pattern: "cp -fL.*claude.*usr/local/bin"
---

<objective>
修复 Dockerfile 中 install.sh 安装 claude 后产生的符号链接被 `mv` 移动导致容器重建后软链断裂的问题。

Purpose: install.sh 将 claude 安装在 `~/.local/share/claude/versions/{version}/`，`~/.local/bin/claude` 是指向该目录的符号链接。当前 Dockerfile 用 `mv` 将软链移到 `/usr/local/bin/claude`，真实二进制留在 `~/.local/share/claude/versions/`（不在任何 volume 挂载中）。容器 stop + rm + run 后 overlay 层重建，`~/.local/share/claude/` 丢失，软链断裂 → `No such file or directory`。

Output: Dockerfile 中 install.sh 路径改为用 `cp -fL` 跟随符号链接复制真实二进制到 `/usr/local/bin/claude`，确保它是独立的 ELF 文件。
</objective>

<execution_context>
@/workspace/Desktop/cloud-cli-proxy/.claude/get-shit-done/workflows/execute-plan.md
</execution_context>

<context>
@deploy/docker/managed-user/Dockerfile
@deploy/docker/managed-user/entrypoint.sh

## 当前问题代码（Dockerfile:147-157）

```dockerfile
    CLAUDE_BIN=""; \
    for candidate in /usr/local/bin/claude "${HOME}/.local/bin/claude" /root/.local/bin/claude; do \
      if [ -x "${candidate}" ]; then CLAUDE_BIN="${candidate}"; break; fi; \
    done; \
    if [ -z "${CLAUDE_BIN}" ]; then \
      echo "Claude binary not found in /usr/local/bin, ${HOME}/.local/bin, or /root/.local/bin" >&2; exit 1; \
    fi; \
    if [ "${CLAUDE_BIN}" != "/usr/local/bin/claude" ]; then \
      mv "${CLAUDE_BIN}" /usr/local/bin/claude; \
    fi; \
    chmod +x /usr/local/bin/claude; \
    claude --version
```

问题：`mv` 移动的是符号链接文件本身，不是它指向的真实二进制。install.sh 创建的 `~/.local/bin/claude` 是软链，真实二进制在 `~/.local/share/claude/versions/{version}/claude`。

## entrypoint.sh 中的 volume 挂载

entrypoint.sh 只持久化 `/var/lib/claude-persist`（用于 `.claude` 和 `.cache/claude` 配置），`~/.local/share/claude/` 不在任何 volume 中。容器重建后该目录丢失。

## GitHub release fallback 路径（Dockerfile:130-145）

直接下载 tar.gz 到 `/tmp`，解压后 `mv /tmp/claude /usr/local/bin/claude`，已经是真实文件，不需要改。
</context>

<tasks>

<task type="auto">
  <name>Task 1: 修复 install.sh 路径的软链复制逻辑</name>
  <files>deploy/docker/managed-user/Dockerfile</files>
  <action>
修改 Dockerfile:147-157 的 claude 二进制定位与复制逻辑：

1. 将遍历候选路径后的复制逻辑从 `mv "${CLAUDE_BIN}" /usr/local/bin/claude` 改为 `cp -fL "${CLAUDE_BIN}" /usr/local/bin/claude`
2. `-L` 选项强制 `cp` 跟随符号链接，复制真实的目标文件内容，而不是复制符号链接本身
3. `-f` 选项强制覆盖已存在的目标文件
4. 保留 `chmod +x /usr/local/bin/claude` 和 `claude --version` 验证
5. 保留 GitHub release fallback 路径不变（它已经是真实文件）

具体改动：
- 第 154-156 行：
  ```dockerfile
  if [ "${CLAUDE_BIN}" != "/usr/local/bin/claude" ]; then \
    cp -fL "${CLAUDE_BIN}" /usr/local/bin/claude; \
  fi; \
  ```

边界处理：
- `cp -fL` 在源文件不存在时报错，但此处源文件已在前面的 `-x` 检查中确认存在且可执行
- `cp -fL` 会覆盖目标文件，适合 build 时覆盖
- 如果 `cp` 失败（极少见），由于 `set -e` 整个 RUN 会失败，build 不会继续，是安全行为
  </action>
  <verify>
    <automated>grep -n "cp -fL" deploy/docker/managed-user/Dockerfile</automated>
  </verify>
  <done>Dockerfile 中 install.sh 路径的复制逻辑改为 `cp -fL`，GitHub release fallback 路径未改动，`claude --version` 验证保留</done>
</task>

<task type="auto">
  <name>Task 2: 验证 Dockerfile 语法和逻辑完整性</name>
  <files>deploy/docker/managed-user/Dockerfile</files>
  <action>
1. 确认 Dockerfile 中修改后的 RUN 段语法正确，反斜杠续行符完整
2. 确认 `cp -fL` 只出现在 install.sh 路径（147-157 区域），不影响 GitHub release fallback 路径（130-145 行）
3. 确认 `claude --version` 验证仍然在 RUN 段末尾
4. 快速检查整个 Dockerfile 是否有其他 `mv` 操作 claude 二进制的地方
  </action>
  <verify>
    <automated>grep -n "mv.*claude\|cp.*claude" deploy/docker/managed-user/Dockerfile</automated>
  </verify>
  <done>修改后的 Dockerfile 中，install.sh 路径使用 `cp -fL`，GitHub fallback 路径仍使用 `mv`，`claude --version` 验证保留，无其他 claude 二进制移动操作</done>
</task>

</tasks>

<verification>
- `grep -n "cp -fL" deploy/docker/managed-user/Dockerfile` 返回包含 `cp -fL "${CLAUDE_BIN}" /usr/local/bin/claude` 的行
- `grep -n "mv.*claude" deploy/docker/managed-user/Dockerfile` 只在 GitHub release fallback 路径（144 行）出现
- Dockerfile 语法完整，反斜杠续行符无遗漏
</verification>

<success_criteria>
- Dockerfile 中 install.sh 成功安装后，claude 二进制通过 `cp -fL` 复制为真实 ELF 文件到 `/usr/local/bin/claude`
- 容器重建后 `/usr/local/bin/claude` 仍可正常执行，不再出现 `No such file or directory`
- GitHub release fallback 路径行为不变
- build 时的 `claude --version` 验证保留
</success_criteria>

<output>
After completion, create `.planning/quick/260506-urq-dockerfile-claude/260506-urq-SUMMARY.md`
</output>
