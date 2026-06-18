---
status: in_progress
---

# Quick Task 260618-lpg: 收窄 DNS hijack 并阻断公网 DNS

## 目标

修复 `v4.2.7` 中 DNS 路由规则过宽的问题：本地 DNS stub 流量应由 `hijack-dns` 接管，用户主动访问公网 DNS（如 `dig @8.8.8.8`）必须被拒绝，避免 `net.dns_leak` 校验失败。

## 背景

`v4.2.7` 已修复 `type: "dns"` inbound 导致的 `sing-box` 配置解码失败，但线上启动推进到网络校验后又失败在 `public DNS @8.8.8.8 not blocked`。原因是 route 规则中 `protocol=dns + hijack-dns` 对所有 DNS 包生效，导致公网 DNS 探针也被 sing-box DNS 模块成功应答。

## 任务

1. 补回归测试，要求 `hijack-dns` 仅匹配 `dns-direct` inbound。
2. 补回归测试，要求其他 DNS 协议流量进入 `reject` 规则。
3. 修复 `buildContainerRoute` 的规则顺序。
4. 更新 `CHANGELOG.md` 的 `v4.2.8` 条目。
5. 走 PR、合并、tag `v4.2.8`，远端拉镜像并验证容器启动通过。

## 验收

- `go test ./internal/network -run TestBuildContainerSingBoxConfig_DNSHijackScopedToStubAndRejectsOtherDNS -count=1` 先失败后通过。
- `go test ./internal/network -count=1` 通过。
- 远端 `dig @8.8.8.8 example.com` 在受管容器内返回非零退出码。
- 控制面网络校验不再报 `net.dns_leak`。
