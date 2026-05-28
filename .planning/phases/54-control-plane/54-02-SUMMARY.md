---
phase: 54-control-plane
plan: "02"
requirements-completed: [CTRL-03]
subsystem: infra
tags: [docker, sing-box, networking, config, single-container, v4.0]

# Dependency graph
requires:
  - plan: 54-01
    provides: SingBoxConfigDir + PrepareGateway 骨架（stub writeContainerSingBoxConfig）+ PrepareHost verifyWorkerNetwork + CleanupHost 单职责
provides:
  - container_singbox_config.go — buildContainerSingBoxConfig / buildContainerTunInbound / buildContainerRouteRules
  - writeContainerSingBoxConfig 实装 — 文件权限 root:singbox 0640，严格对齐 entrypoint hard-assert
  - isChownPermissionError — darwin chown 降级判断
  - container_singbox_config_test.go — 7 条单测覆盖 config 生成 + 权限对齐 + chown 降级
affects: [54-03, 54-04, 55]

# Tech tracking
language: go
files_changed:
  - source: internal/network/container_singbox_config.go
    state: created
    changes:
      - buildContainerSingBoxConfig — 渲染容器内 sing-box config（tun inbound 172.19.0.1/30 + DNS stub + route final proxy-out + proxy server IP direct 回环规则）
      - buildContainerTunInbound — tun inbound（address/auto_route/strict_route/stack=system/sniff）
      - buildContainerRouteRules — 4 条精简规则（sniff → hijack-dns → proxy_ip direct → private direct）
  - source: internal/network/container_proxy_provider.go
    state: modified
    changes:
      - writeContainerSingBoxConfig — 替换 Plan 54-01 stub，完整实现：resolve proxy IP → build config → MkdirAll → WriteFile(0600) → Chown(0:9000) → Chmod(0640)
      - PrepareGateway chown 降级 — darwin 非 root 时 isChownPermissionError → Warn 不阻断
      - isChownPermissionError — 字符串子串匹配 "chown" + "operation not permitted"
      - singboxGroupGID = 9000 常量
  - source: internal/network/container_singbox_config_test.go
    state: created
    changes:
      - TestBuildContainerSingBoxConfig_TunInboundAddress
      - TestBuildContainerSingBoxConfig_RouteFinalProxyOut
      - TestBuildContainerSingBoxConfig_DirectForProxyServerIP
      - TestBuildContainerSingBoxConfig_NoWhitelistRuleSet
      - TestBuildContainerSingBoxConfig_DNSStubServers
      - TestBuildContainerSingBoxConfig_NoEndpointIndependentNAT
      - TestIsChownPermissionError — 6 个子用例（chown_eperm / chown_eacces / mkdir_eperm / write_eperm / chmod_eperm / nil）

# Completed tasks
- [x] T1 — 新建 container_singbox_config.go
- [x] T2 — 实装 writeContainerSingBoxConfig 真实逻辑
- [x] T3 — 单元测试（容器内 config 生成，4 条用例）
- [x] T4 — 单元测试（文件权限 + chown 降级）
- [x] T5 — PrepareGateway chown 错误降级处理

# Key decisions
- tun address 保留 172.19.0.1/30 与 v3.5 兼容（容器私有 IP，不冲突）
- v4.0 不引入 v3.5 whitelist rule-set（bypass 白名单留 v4.1）
- chown 失败 darwin 降级 WARN（非 root）| prod Linux root → entrypoint hard-assert 兜底
- singboxGroupGID 写死 9000 numeric，不对齐 host getent

# Dev notes
- 本地 macOS `go test ./internal/network/` 全部通过（chown 降级路径已覆盖）
- entrypoint `start_singbox_or_die` hard-assert（perm/owner/group/uid）保证 Linux prod 正确性

# Self-Check: PASSED
- [x] 7 条单测全部 PASS（TestBuildContainerSingBoxConfig_* × 6 + TestIsChownPermissionError × 1）
- [x] 文件权限 root:singbox 0640 严格对齐 entrypoint hard-assert
- [x] chown 降级仅 WARN 不阻断 darwin 本地开发
- [x] container_singbox_config.go 无 build tag（darwin 可编译）
