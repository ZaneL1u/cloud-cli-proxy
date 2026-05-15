# Phase 53 Smoke Tests

本地烟测 Phase 53 镜像与 entrypoint 启动序列。

## 跑测试

```bash
# 1. 构建镜像（Plan 53-01 + 53-02 改造完成后）
docker build -t managed-user:v4-dev -f deploy/docker/managed-user/Dockerfile .

# 2. 跑烟测
bash tests/phase53/smoke.sh
```

或通过 Makefile 入口：

```bash
make phase53-smoke
```

期望: 6 条 T-53-* 断言全部 OK。

## 不是 e2e

Phase 55 才完整接入 v3.6 e2e harness（testcontainers-go + testify/suite）。
本目录是 Phase 53 开发期的本地一键自测，不进 CI。

## 与 Plan 关系

| 断言 | 验收 PLAN |
|---|---|
| T-53-1 (tun0 + uid=9000) | 53-01 IMG-02 / 53-02 EP-02 |
| T-53-2 (config rm) | 53-02 D-V4-2 |
| T-53-3 (tun 接管) | 53-02 EP-03 / NET-01 |
| T-53-4 (空 cap + 无 sudo) | 53-01 IMG-04 / 53-02 NET-04 |
| T-53-5 (fail-closed ≤3s) | 53-02 EP-04 / NET-03 |
| T-53-6 (restart on-failure) | 53-02 + docker daemon 行为 |

## fixture 说明

`fixtures/test-singbox-config.json` 是最小可用 sing-box config，使用 `direct` outbound（直连，无上游代理），仅用于
"sing-box 能起来 + tun 接管 + curl 走 tun" 的烟测。**不验证出口 IP 是上游代理 IP**——那需要真实 outbound 配置，
Phase 55 e2e 才覆盖。

T-53-3 通过两条独立 oracle 共同断言流量确实走 tun：

1. `ip route show table all` 默认路由 `dev=tun0`
2. `curl https://api.ipify.org` 在容器内能返回（tun 转发链路 OK）

## 平台限制

- macOS / Windows 上 docker desktop 通过虚拟机跑 linux/amd64，`--device /dev/net/tun` 与 `--cap-add NET_ADMIN`
  在 docker desktop 4.x 上一般可用，但 sing-box tun stack 的网络性能与计时（T-53-5 ≤3s 关停）可能略有偏差。
- T-53-6 的 `--restart=on-failure` 行为依赖 docker daemon；linux 原生最稳。
- 任一项在本地 desktop 跑不通，归 deferred-to-Phase-55-CI（在 ubuntu runner 跑）。
