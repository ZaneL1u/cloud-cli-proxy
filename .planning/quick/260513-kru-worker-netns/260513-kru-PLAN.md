---
quick_id: "260513-kru"
slug: "worker-netns"
description: "修复 worker netns 获取失败（容器状态检查 + 重试）"
mode: quick
created: 2026-05-13
---

# 修复 worker netns 获取失败

## 问题

创建主机时，worker 容器在 `configureWorkerEgress`（docker exec 成功）之后、`applyWorkerFirewall` 之前死亡，导致 `GetContainerNetNS` → `netns.GetFromPid(pid)` 失败：

```
get netns from pid 100202: no such file or directory
```

## 根因

`docker network disconnect -f bridge` 后容器经历网络断连，`docker exec` 修改路由/DNS，但紧接着调用 `GetContainerNetNS` 时容器 PID 已消失。可能是 Docker 状态更新与进程生命周期之间的竞态，或容器 init 进程在网络重配后退出。

## 修复方案

### Task 1: namespace.go — GetContainerNetNS 增加重试和状态检查

**文件**: `internal/network/namespace.go`

修改 `GetContainerNetNS` 函数：

1. 在获取 PID 后、调用 `netns.GetFromPid` 前，先验证容器 Running 状态
2. 添加最多 5 次重试，每次间隔 300ms
3. 最终失败时，通过 `docker inspect` 读取容器 `State.Status`、`State.ExitCode` 并嵌入错误信息，便于排障

### Task 2: container_proxy_provider.go — configureWorkerEgress 后加短暂等待

**文件**: `internal/network/container_proxy_provider.go`

在 `configureWorkerEgress` 调用成功后、`applyWorkerFirewall` 调用前加 `time.Sleep(500 * time.Millisecond)`，给 Docker 时间稳定容器进程状态。

## 验证

- `go vet ./internal/network/...` 通过
- `go build ./internal/network/...` 通过
- `GOOS=linux go test ./internal/network/ -c -o /dev/null` 编译通过

## 约束

- 不动任何函数签名
- 不动防火墙规则逻辑
- 只增加健壮性和排障信息
