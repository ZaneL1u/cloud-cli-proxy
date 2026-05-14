---
phase: 49-leak-defense
plan: 04
title: LEAK-04 IPv6 阻断 (SUMMARY)
status: implemented
leak: LEAK-04
build_tag: "e2e && linux"
created: 2026-05-14
---

# Phase 49 Plan 04 SUMMARY: LEAK-04 IPv6 阻断

## 实际实现

- **双保险断言**：
  - `g.ReadProcFile(ctx, "/proc/sys/net/ipv6/conf/all/disable_ipv6") == "1"`。
  - `g.ReadProcFile(ctx, "/proc/sys/net/ipv6/conf/default/disable_ipv6") == "1"`。
- **探测方法**：`g.CurlIPv6(ctx, "https://ipv6.google.com")` 必须 Blocked
  （exit 6 / 7 / 28 / unreachable）。
- **抓包 oracle**（条件性）：通过 `docker inspect -f '{{.NetworkSettings.GlobalIPv6Address}}'`
  拿 worker IPv6；为空 → 跳过抓包断言（IPv6 stack 整个被关，是更强约束）；
  非空 → BPF `ip6 and src host <ipv6>` 必须 0 包。
- **裁决**：任一双保险 fail → t.Fatalf；curl 未阻断 → t.Fatalf；抓包 > 0 → t.Fatalf。

## 与 Plan 偏差

无；按 Plan 走 双 disable_ipv6 + curl + 条件性抓包。新增本地 helper `inspectContainerIPv6`
（仅在本用例文件，不污染 helpers_linux.go）。

## 实际命令 / 工具

- `cat /proc/sys/net/ipv6/conf/{all,default}/disable_ipv6`。
- `curl -6 -sS --max-time 3 https://ipv6.google.com`。
- `docker inspect -f '{{.NetworkSettings.GlobalIPv6Address}}' <worker>`。

## 单测覆盖（darwin）

复用 shared `ClassifyLeakProbe`；fixture：`curl_ipv6_unreachable.txt` /
`proc_disable_ipv6_one.txt` / `proc_disable_ipv6_zero.txt`。

## Phase 51 GAP

无（本 plan 期望 PASS）；worker.go:229-230 已显式
`--sysctl net.ipv6.conf.{all,default}.disable_ipv6=1`，配合 worker netns nft
IPv6 表 input6 / output6 policy=drop（firewall_helpers.go:24-30）双层 fail-closed。
