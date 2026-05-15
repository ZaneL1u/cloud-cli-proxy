---
phase: 53-entrypoint
created: 2026-05-16T03:50:00Z
source: 53-REVIEW.md (medium/low findings)
status: deferred
items: 11
---

# Phase 53 Tech Debt 登记

来源：`53-REVIEW.md` 中 6 条 MEDIUM + 5 条 LOW 发现。Phase 53 的 CRITICAL / HIGH（CR-01、HI-01..03）+ 2 项 verifier gap（GAP-1/2）已在本轮 fix 中关闭；以下条目转为 tech debt，按 ID 列出关联的后续 phase 与简短建议，**本 phase 不修代码**。

| ID | 标题 | 关联 phase | 建议 / 处置 |
| -- | ---- | ---------- | ----------- |
| ME-01 | smoke.sh `sudo chown` 没 `-n`，开发期会卡密码 prompt 且 `\|\| true` 吞错误 | Phase 55（CI 收敛 smoke 行为）| 改 `sudo -n chown root:9000 "$TMP_CONFIG"` 并去掉 `\|\| true`，失败 fail-fast。CR-01 fix 已把 chown target 改为 `root:9000`，本条仅缺 `-n` + 错误处理收敛。 |
| ME-02 | smoke T-53-4 缺 `ip link set tun0 down` EPERM 断言 | — | **已在 GAP-2 fix（commit `fix(53-GAP-2)`）关闭**，本条留作回溯指针。 |
| ME-03 | Dockerfile apt 缺 `dnsutils`（dig 工具） | — | **已在 GAP-1 fix（commit `fix(53-GAP-1)`）关闭**，本条留作回溯指针。 |
| ME-04 | `apply_nft_or_die` 二次 verify 仅证 nft table 可见，不证 hook 已绑定 | Phase 55（运行时 oracle）| 加一条 dummy probe（探测无主机 IP 触发 drop counter）确认 hook 真生效。需先有完整镜像 + 真容器跑起来，配合 Phase 55 CI smoke 一起验证。可在 53-CONTEXT 标 known limitation。 |
| ME-05 | `lock_resolv_conf` 没先 `chattr -i`，旧 immutable bit 会让 `cat >` 失败 | Phase 54（host-agent 注入）/ Phase 55（CI 验证）| `chattr +i` 之前先 `chattr -i /etc/resolv.conf 2>/dev/null \|\| true`。Phase 54 host-agent 接管 resolv.conf 注入时如果选 immutable 持久化策略，本条更需对齐。 |
| ME-06 | `start_singbox_or_die` waitFor 期间不验证 sing-box 实际 uid | — | HI-03 fix 已在 setpriv 切换后于 waitFor 内加 `sb_uid != 9000 → exit 1` hard assertion，**本条事实上已被 HI-03 一并关闭**，留作回溯指针。 |
| LO-01 | `shred -u` 在 overlay2 上无法保证物理擦除 | Phase 54（host-agent threat model）| 文档化即可：在 53-CONTEXT D-V4-2 注明"shred 在 overlay2 仅 best-effort，secure deletion 不在 v4.0 威胁模型内"。Phase 54 host-agent 注入侧补"config 读完即 unlink，不依赖 shred 物理擦除"语义。 |
| LO-02 | 镜像保留 `sudo` 包（SUID root binary 留攻击面） | Phase 54 / Phase 55（镜像收敛）| Dockerfile L25-57 删 `sudo` 行；或单独 `RUN apt-get purge -y sudo`。验收：`docker run --rm managed-user:vX which sudo` 返回非零。需配合 v3.x 残留路径回归测试一起做。 |
| LO-03 | sing-box stdout/stderr 没重定向到日志文件 | Phase 55（CI 排障可观测性）| `mkdir -p /var/log/sing-box && chown singbox:singbox` + `setpriv ... sing-box ... >>/var/log/sing-box/stdout.log 2>>/var/log/sing-box/stderr.log &`。或在生产 config `log.output` 字段配置（更优，与 host-agent 注入策略对齐 → Phase 54）。 |
| LO-04 | nft 显式 drop 仅覆盖 IPv4 DNS，IPv6 DNS 仅靠 policy drop 兜底无 counter | Phase 56（leak 测试 / 运营可观测性）| `default-deny.nft` 追加 `meta l4proto udp udp dport 53 ip6 daddr != ::1 counter drop` 与对应 TCP 规则，让 IPv6 DNS 尝试有专属 counter。entrypoint 已 sysctl disable_ipv6，本条仅补可观测性，不影响安全性。 |
| LO-05 | `assert_tmux_version` case 模式 `[4-9].*` 与 `3.4*..3.9*` 共存易误读 | Phase 55（代码可读性 / refactor）| 改为 major.minor 数字提取 + `(( major < 3 ))` 比较。非主线，可选。 |

## 处置原则

- ME-02 / ME-03 / ME-06 已在本轮 fix 顺带关闭，仅在表中保留 ID → commit 映射，便于将来 audit 回溯。
- 其余 8 条按关联 phase 分散：Phase 54（host-agent）3 条、Phase 55（CI / 镜像收敛 / 排障）4 条、Phase 56（leak 测试 / 可观测性）1 条。
- 本文件不引入新代码改动；所有条目在对应 phase plan/discuss 阶段被显式拉入 PLAN frontmatter 时正式启动。

---

_Created: 2026-05-16T03:50:00Z_
_Source: 53-REVIEW.md medium/low findings_
