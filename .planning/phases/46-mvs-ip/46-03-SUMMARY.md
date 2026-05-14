---
phase: 46-mvs-ip
plan: 03
subsystem: tests/e2e
tags: [mvs-03, dns, or-semantics, classify]
provides:
  - classify-dns-result-pure-function
  - dns-test-skeleton
requires:
  - tests/e2e/helpers.go (ClassifyDNSResult / DNSProbeResult，46-01 已落)
affects:
  - tests/e2e/dns_test.go (新增)
tech-stack:
  added: []
  patterns:
    - "OR 语义：Tunneled / Denied 任一 PASS"
    - "stderr 关键字白名单：connection refused / timed out / network is unreachable 等 8 个 musl/glibc 通用文案"
key-files:
  created:
    - tests/e2e/dns_test.go
  modified: []
decisions:
  - "ClassifyDNSResult 在 46-01 一次性落地；本 plan 只新增主用例 dns_test.go"
  - "dnsDenyKeywords 列入 'timeout' 单独条目（不仅 'timed out'），覆盖 BusyBox getent 的精简文案差异"
  - "HTTPS 握手二次校验放在 DNSResultTunneled 分支：getent 成功 + HTTPS 握手不通 = 仍 Tunneled（防火墙未拒绝 DNS 但拒绝 HTTPS）。本 plan 不再细分，Phase 49 防泄漏对抗扩展"
metrics:
  duration: 约 10 分钟
  tasks_completed: 3/3
  files_modified: 1
  completed_at: 2026-05-14
requirements_satisfied:
  - MVS-03 (ClassifyDNSResult 纯函数完整 + 用例骨架就位)
requirements_partial:
  - MVS-03 Linux 真机断言 + nft counter dump（deferred-to-CI / Phase 52 OBS-02）
---

# Phase 46 Plan 03 Summary: DNS 强制走 tun OR 防火墙拒绝

## One-liner

新增 `tests/e2e/dns_test.go` 主用例，调 46-01 已就位的 `ClassifyDNSResult(exitCode, stderr) -> DNSProbeResult`，在 worker 容器内对 `cloudflare.com` 做 `getent hosts` probe，Tunneled / Denied 任一成立即 PASS，Unknown → Fail。

## 实际产出

| 文件 | 性质 | 关键内容 |
|------|------|----------|
| `tests/e2e/dns_test.go` | 新建 | `//go:build e2e && linux`；DNSSuite + TestDNS_TunOrFirewallDeny；OR 语义判定 + DNSResultTunneled 时附加 HTTPS 握手校验 |

## 验证结果

- `go build ./tests/e2e/...`（darwin）exit 0 ✓
- `go test ./tests/e2e/ -run "HelpersClassifyDNS|HelpersDNSProbeResult" -count=1`（darwin）7 PASS ✓
- `GOOS=linux go vet -tags='e2e linux' ./tests/e2e/...` 干净 ✓

## 与 PLAN 偏差

- 解析命令选 `getent hosts cloudflare.com`（alpine + busybox + glibc/musl 通用），不强依赖 `dig`/`nslookup`，比 PLAN 03 描述更稳健。
- nft counter dump 列入 deferred-to-CI（Phase 52 OBS-02）；当前 artifact dumper 已注入，失败时会建占位目录与 README。

## 风险与遗留

- 解析失败但 stderr 文案不在 dnsDenyKeywords 中时会落到 `Unknown` → Fail；如 CI 上 musl 文案演进，需要在 helpers_test 中补单测 + 在 `dnsDenyKeywords` 列表追加。
- HTTPS 握手 5s 超时；CI 出口 cert chain 受限时可能 false fail，需 Phase 49 评估。

## 给后续 plan 的接口契约

- `DNSProbeResult` 与 `ClassifyDNSResult` 暴露给 Phase 49 防泄漏对抗复用；新增 `DNSResultLeaked` 分类时不破坏现有 enum 顺序。
