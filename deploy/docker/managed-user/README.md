# 受管用户镜像说明

该镜像是“一用户一主机”模型下的标准模板容器，提供默认 SSH 工作环境、基础 Shell 工具以及预装的 `claude code`。

运行时约定如下：

- 主目录持久化挂载点固定为 `/workspace`
- 默认用户固定为 `workspace`
- 控制面与 host-agent 必须统一读取 `image.lock` 中的同一个镜像全名
- 默认重建模式是 `preserve-home`，即重建系统层但保留 `/workspace`
- `factory_reset_mode: wipe-/workspace` 仅作为后续显式“清空所有数据并重建”入口的契约，不在 Phase 1 自动执行

Phase 2 只允许在这个模板旁边新增网络准备钩子接口，不在本计划内落地隧道、出口 IP 绑定或其他网络强约束实现。

## 容器内运维脚本

- `restart-vnc`：重启 KasmVNC + 桌面进程（不重建容器）。
- `claude`：默认包装为基于 `tmux` 的持久会话模式：
  - 会话名按 `pwd -P` 计算，同目录重复执行会复用同一 Claude 进程。
  - 并发打开同目录时会自动收敛到同一个会话（避免竞争创建多个实例）。
  - SSH 断开不会结束 Claude 进程。
  - 临时关闭该行为：`CLAUDE_NO_TMUX=1 claude ...`
  - 可选自定义 tmux socket：`CLAUDE_TMUX_SOCKET=/path/to/socket claude`
