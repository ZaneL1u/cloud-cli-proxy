---
status: complete
---

# Quick Task 260618-lpg 总结：收窄 DNS hijack 并阻断公网 DNS

## 结果

修复 `v4.2.7` 中 `hijack-dns` 规则过宽的问题。本地 DNS stub inbound 继续由 sing-box DNS 模块接管，用户主动访问公网 DNS（如 `dig @8.8.8.8`）则命中 fallback DNS reject 规则。

## 修改

- `internal/network/container_singbox_config.go`
  - `hijack-dns` 规则增加 `inbound: "dns-direct"`。
  - 新增 `protocol: "dns" + action: "reject"` fallback 规则。
- `internal/network/container_singbox_config_test.go`
  - 新增回归测试，锁定 DNS hijack 只能匹配本地 stub inbound。
  - 新增回归测试，锁定非 stub DNS 流量必须 reject。
- `CHANGELOG.md`
  - 新增 `v4.2.8` 修复条目。

## 验证

- 红灯验证：
  - `go test ./internal/network -run TestBuildContainerSingBoxConfig_DNSHijackScopedToStubAndRejectsOtherDNS -count=1`
  - 修复前失败，命中无条件 `protocol=dns + hijack-dns`。
- 绿灯验证：
  - `go test ./internal/network -run TestBuildContainerSingBoxConfig_DNSHijackScopedToStubAndRejectsOtherDNS -count=1`
  - `go test ./internal/network -count=1`
- 远端热验证：
  - 临时应用同等路由规则后，受管容器内 `dig @8.8.8.8 example.com` 返回非零退出码。
  - `/etc/resolv.conf` 仍指向 `127.0.0.1`，普通域名解析正常。
