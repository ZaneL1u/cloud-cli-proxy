# Mutagen 同步策略深度研究

**研究日期:** 2026-04-30
**研究范围:** mutagen-io/mutagen (Go 文件同步引擎)
**置信度:** HIGH (官方文档 + 源码验证)

---

## 目录

1. [同步模式语义](#1-同步模式语义)
2. [删除冲突处理](#2-删除冲突处理)
3. [删除防抖动与Staging机制](#3-删除防抖动与staging机制)
4. [扫描失败保护行为](#4-扫描失败保护行为)
5. [快照与状态机机制](#5-快照与状态机机制)
6. [对 Cloud CLI Proxy Hot-Sync 的改进建议](#6-对-cloud-cli-proxy-hot-sync-的改进建议)
7. [关键概念定义](#7-关键概念定义)
8. [来源引用](#8-来源引用)

---

## 1. 同步模式语义

Mutagen 提供四种同步模式，构成一个 2x2 矩阵：

| 方向 | Safe (保守) | Auto-resolved (强制) |
|------|-------------|----------------------|
| **双向** | `two-way-safe` | `two-way-resolved` |
| **单向** | `one-way-safe` | `one-way-replica` |

### 1.1 `two-way-safe` (默认)

**语义:** 双向同步，两端权重相等。冲突仅在"不导致数据丢失"时自动解决；不可自动解决的冲突被记录到会话状态，可通过 `mutagen sync list` 查看。

**关键行为:**
- 一端修改、另一端删除对应内容时：修改方获胜（修改覆盖删除），因为"deletions can be overwritten"
- 两端都修改同一文件时：产生冲突，需要人工解决
- 冲突解决方式：删除你希望输掉冲突的那一端的内容，Mutagen 会自动将另一端的内容同步过来

**适用场景:** 开发环境双向同步，需要保留人工干预能力。

### 1.2 `two-way-resolved`

**语义:** 双向同步，但 alpha 端自动赢得所有冲突，包括 alpha 的删除会覆盖 beta 的修改或创建。

**关键行为:**
- 无冲突概念——所有分歧都以 alpha 为准
- alpha 删除的文件，即使 beta 有修改，也会被删除
- beta 的新创建文件，如果 alpha 没有，会被删除

**适用场景:** 有明确主从关系的双向同步，alpha 是权威源。

### 1.3 `one-way-safe`

**语义:** 单向同步（alpha -> beta），但保护 beta 端的独立修改。

**关键行为:**
- beta 端的删除会被 alpha 的内容覆盖（文件会"回来"）
- beta 端的修改和创建不会被 alpha 覆盖
- beta 端额外的非冲突内容被忽略（不会同步回 alpha）
- 冲突被记录到会话状态

**适用场景:** 从主仓库向工作区推送更新，但允许工作区有本地实验性修改。

### 1.4 `one-way-replica`

**语义:** beta 成为 alpha 的精确副本。

**关键行为:**
- beta 的任何修改或额外内容都会被立即覆盖或删除
- 无冲突概念
- 等同于 `rsync --delete` 的持续运行版本

**适用场景:** 部署场景，目标端必须严格镜像源端。

---

## 2. 删除冲突处理

### 2.1 核心原则: "Deletions Can Be Overwritten"

Mutagen 的冲突解决遵循一个基本原则：**删除可以被覆盖**。这意味着：

- 修改 vs 删除 -> 修改获胜
- 创建 vs 删除 -> 创建获胜
- 删除 vs 删除 -> 双方达成一致（无冲突）

### 2.2 双向模式下的删除冲突处理

 reconciliation 算法在 `handleDisagreementBidirectional` 中处理删除冲突：

**步骤 1: 提取可同步部分**
```
alphaSync = alpha.synchronizable()
betaSync = beta.synchronizable()
alphaDiff = diff(path, ancestor, alphaSync)
betaDiff = diff(path, ancestor, betaSync)
```

**步骤 2: 检查纯删除情况**
- `extractNonDeletionChanges(alphaDiff)` 过滤出非删除变更
- 如果 `len(alphaDiffNonDeletion) == 0 && len(betaDiffNonDeletion) == 0`：
  - 双方都是纯删除 -> 将完整删除传播到部分删除的那一侧
  - 例如：一侧完全删除了目录，另一侧只删除了部分子内容 -> 传播完整删除

**步骤 3: 一侧纯删除、另一侧有修改**
- 如果只有一侧是纯删除：
  - 将内容从有修改的一侧传播到纯删除的一侧
  - **这是 Mutagen 的人工冲突解决机制的基础**：删除你希望输掉的那一侧，Mutagen 会自动把另一侧的内容同步过来
  - 特别地，"if one side has deleted a directory and the other has created or modified content in that directory... we'll repropagate the entire directory back to the deleted side" —— 这是有意为之，目的是"avoid a conflict and preserve the on-disk 'context' for newly created content"

**步骤 4: 双方都有非删除变更**
- `two-way-safe`: 记录冲突
- `two-way-resolved`: alpha 强制覆盖 beta

### 2.3 单向模式下的删除处理

**`one-way-safe`:**
- alpha 删除 -> 传播到 beta（beta 的文件被删除）
- beta 删除 -> 被 alpha 的内容覆盖（文件"回来"）
- 特殊处理：如果 alpha 是 nil/untracked 且 ancestor/beta 都不是目录，"nil out the ancestor" 并保留 beta 的内容

**`one-way-replica`:**
- alpha 的任何变更（包括删除）都精确复制到 beta
- beta 的独立删除会被恢复

---

## 3. 删除防抖动与 Staging 机制

### 3.1 重要结论: Mutagen 没有"删除延迟"机制

**研究发现:** Mutagen 的 staging 机制仅用于**原子传输**（文件写入时的临时目录），不用于删除操作的延迟或防抖。

### 3.2 Staging 模式 (仅用于传输原子性)

Mutagen 提供三种 staging 模式，控制文件在同步过程中临时存放的位置：

| 模式 | 行为 | 适用场景 |
|------|------|----------|
| `"mutagen"` (默认) | 在 `~/.mutagen` 目录中 staging，完成后原子移动到同步根目录 | 通用场景 |
| `"neighboring"` | 在同步根目录旁边的隐藏临时目录中 staging | 同一文件系统，避免跨 FS 移动 |
| `"internal"` | 在同步根目录内部的隐藏临时目录中 staging | 需要保留文件系统边界 |

**配置方式:**
```yaml
sync:
  defaults:
    stageMode: "mutagen"
```

**关键洞察:** Staging 仅影响"文件如何被写入目标端"，不影响"何时执行删除"。删除操作在 reconciliation 决策后立即执行，没有延迟窗口。

### 3.3 与 Cloud CLI Proxy Hot-Sync 的对比

| 机制 | Mutagen | Cloud CLI Proxy (Phase 36-37) |
|------|---------|------------------------------|
| 删除保护 | 无延迟，依赖安全机制 halt | 大文件熔断 (50MB+) |
| 传输原子性 | Staging + 原子重命名 | SFTP 直接写入 |
| 删除冲突解决 | 三向合并 + 模式策略 | 单向 (remote -> local) |
| 批量删除保护 | Root deletion/emptying halt | 无 (需评估添加) |

---

## 4. 扫描失败保护行为

### 4.1 扫描错误分类

Mutagen 的 `Scan()` 方法返回三个值：
```go
Scan(ctx context.Context, ancestor *Entry, full bool) (*Snapshot, error, bool)
// 返回值: (snapshot, error, tryAgain)
```

第三个返回值 `tryAgain` (bool) 指示是否为"临时性"错误：
- `tryAgain = true`: 临时错误（如并发修改导致的不一致），应重试
- `tryAgain = false`: 非临时错误（如权限拒绝、路径不存在），应终止

**注意:** 源码中未发现独立的 `IsEphemeralScanError` 函数——错误分类由各个 endpoint 实现在 `Scan()` 方法内部完成，通过 `tryAgain` 返回值传递。

### 4.2 临时错误处理流程

在 `controller.synchronize()` 中：

```go
if alphaScanErr != nil {
    alphaScanErr = fmt.Errorf("alpha scan error: %w", alphaScanErr)
    if !alphaTryAgain {
        return alphaScanErr  // 非临时错误 -> 终止同步循环
    } else {
        // 临时错误 -> 记录错误，跳过 polling，强制重试
        c.stateLock.Lock()
        c.state.LastError = alphaScanErr.Error()
        c.stateLock.Unlock()
        skipPolling = true
        skippingPollingDueToScanError = true
    }
}
```

**连续临时错误时的退避:**
```go
if skippingPollingDueToScanError {
    c.stateLock.Lock()
    c.state.Status = Status_WaitingForRescan
    c.stateLock.Unlock()
    
    select {
    case <-time.After(rescanWaitDuration):  // 5 秒等待
    case <-ctx.Done():
        return errors.New("cancelled during rescan wait")
    }
}
```

### 4.3 关键设计决策

| 决策 | Mutagen 行为 |
|------|-------------|
| 扫描失败时是否使用上一轮快照 | **否** — 不使用旧快照，而是跳过当前周期，等待 `rescanWaitDuration` 后重新扫描 |
| 临时错误是否终止同步会话 | **否** — 仅跳过当前周期，进入 `Status_WaitingForRescan` 状态 |
| 非临时错误是否终止同步会话 | **是** — 返回错误，触发 `run()` 中的重连逻辑 |
| 重试间隔 | `rescanWaitDuration = 5 * time.Second` |

### 4.4 扫描模式

Mutagen 支持两种扫描模式：

| 模式 | 行为 | 适用场景 |
|------|------|----------|
| `accelerated` (默认) | 使用文件系统事件和缓存加速扫描 | 日常同步 |
| `full` | 完整遍历，不使用加速 | 强制一致性检查 |

**关键保证:** "even in cases where `accelerated` scanning returns a slightly outdated synchronization root snapshot, Mutagen's change application algorithms will still detect conflicting changes that might have been missed in the outdated snapshot, so the safety behavior is the same as with `full` scanning."

---

## 5. 快照与状态机机制

### 5.1 快照结构

```go
type Snapshot struct {
    Content               *Entry  // 文件系统树的核心数据
    PreservesExecutability bool   // 权限处理行为元数据
    DecomposesUnicode      bool   // Unicode 规范化行为元数据
}
```

- `Content`: 完整的文件系统树状态（非增量）
- `PreservesExecutability`: 平台是否保留可执行权限
- `DecomposesUnicode`: 平台是否使用 NFD Unicode 分解

### 5.2 三向合并中的 Ancestor (Last-Sync) 状态

Mutagen 的核心算法是**递归三向合并**，使用三个状态：

| 状态 | 含义 | 来源 |
|------|------|------|
| `ancestor` | 上次同步达成的一致状态 | 持久化存档 (archive) |
| `alpha` | alpha 端当前状态 | 实时扫描 |
| `beta` | beta 端当前状态 | 实时扫描 |

**Ancestor 的关键作用:**

1. **变更检测:** `diff(path, ancestor, alpha)` 计算出 alpha 相对于上次同步的变更
2. **冲突检测:** 如果 alpha 和 beta 都相对于 ancestor 发生了不同变更 -> 冲突
3. **"Both Modified Same" 优化:** 如果 alpha == beta 但 ancestor != alpha，说明两端独立做了相同修改 -> 无冲突，直接更新 ancestor
4. **持久化:** ancestor 通过 protobuf 序列化存储在 archive 中，跨会话保持

### 5.3 状态机

控制器 (`controller.go`) 维护一个状态机，包含 14 种状态：

| 状态 | 含义 |
|------|------|
| `Status_Disconnected` | 未连接 |
| `Status_ConnectingAlpha` | 连接 alpha 端 |
| `Status_ConnectingBeta` | 连接 beta 端 |
| `Status_Watching` | 等待文件系统事件 |
| `Status_Scanning` | 扫描文件 |
| `Status_Reconciling` | 协调变更 |
| `Status_Staging` | 传输文件 |
| `Status_Applying` | 应用变更 |
| `Status_WaitingForRescan` | 等待重新扫描 |
| `Status_HaltedOnRootEmptied` | 安全 halt：根目录被清空 |
| `Status_HaltedOnRootDeletion` | 安全 halt：根目录被删除 |
| `Status_HaltedOnRootTypeChange` | 安全 halt：根类型变更 |

**同步循环结构:**
```
run() 循环:
  1. 连接 alpha 端
  2. 连接 beta 端
  3. synchronize() 循环:
     a. Poll() 等待事件或 flush 请求
     b. 扫描 alpha 和 beta
     c. 协调变更 (reconcile)
     d. 传输文件 (stage)
     e. 应用变更 (apply)
  4. 错误处理 -> 自动重连
```

### 5.4 存档持久化

- ancestor 状态通过 protobuf 序列化存储
- archive 包含 `Content` 字段（上次同步的完整快照）
- 会话重启后从 archive 恢复 ancestor，实现增量同步

---

## 6. 对 Cloud CLI Proxy Hot-Sync 的改进建议

基于 Mutagen 的研究，以下建议可用于改进 Cloud CLI Proxy 的 hot-sync 引擎（Phase 36-37 已实现的基础之上）：

### 6.1 建议 1: 引入三向合并替代单向覆盖

**当前状态:** Hot-sync 是单向的（remote -> local hot staging），使用简单的"存在即跳过"逻辑。

**Mutagen 启示:** 三向合并（ancestor + alpha + beta）可以区分"创建"、"修改"和"删除"，避免不必要的传输。

**改进方向:**
- 在 `last-session.json` 中存储上次同步的完整文件列表（作为 ancestor）
- 同步时比较：ancestor vs remote vs local
- 仅在 remote 相对于 ancestor 有变更时才触发传输
- 避免重复传输未变更的文件

**优先级:** MEDIUM — 需要评估存储开销与收益

### 6.2 建议 2: 添加批量删除保护（Safety Mechanisms）

**当前状态:** 无批量删除保护。如果 remote 端大量文件被删除，hot-sync 会同步删除 local hot staging。

**Mutagen 启示:** Mutagen 的三种安全机制（root deletion、root emptying、root type change）可以防止意外批量删除。

**改进方向:**
- 在 `HotSyncEngine` 中添加删除阈值检测
- 如果单次同步中删除文件数超过阈值（如 >50% 的文件），触发 halt 或 warn
- 参考 Mutagen 的 `oneEndpointEmptiedRoot` 逻辑：检测"一端删除了非平凡数量的内容并留下空根目录"

**优先级:** HIGH — 数据安全关键

### 6.3 建议 3: 扫描错误分类与优雅降级

**当前状态:** 扫描失败时行为不明确。

**Mutagen 启示:** 区分临时错误（tryAgain=true）和非临时错误，临时错误时跳过当前周期并延迟重试。

**改进方向:**
- 在 SFTP 扫描时区分：
  - 临时错误：网络抖动、并发修改 -> 记录错误，5秒后重试
  - 非临时错误：权限拒绝、路径不存在 -> 终止同步，输出错误码
- 参考 `rescanWaitDuration = 5 * time.Second` 的退避策略

**优先级:** MEDIUM

### 6.4 建议 4: 冲突解决策略可配置化

**当前状态:** 单向同步，无冲突概念。

**Mutagen 启示:** 四种模式（two-way-safe/resolved, one-way-safe/replica）提供了不同的一致性策略。

**改进方向:**
- 如果未来支持双向同步，可引入模式选择
- 即使单向场景，也可配置：
  - `safe` 模式：本地修改不被覆盖（类似 one-way-safe）
  - `replica` 模式：本地严格镜像 remote（类似 one-way-replica）

**优先级:** LOW — 当前单向场景需求不明确

### 6.5 建议 5: 增量扫描优化

**当前状态:** 每次 hot-sync 轮询可能扫描完整目录。

**Mutagen 启示:** `accelerated` 扫描模式通过缓存和事件加速，只在必要时做完整扫描。

**改进方向:**
- 复用 ColdPromoter 的 inotify 基础设施做增量扫描
- 维护"脏路径"集合，只扫描变更的文件/目录
- 参考 Mutagen 的 `dirtyPaths` 机制："we add any re-check path as well as any parent component of any re-check path"

**优先级:** HIGH — 与现有 inotify 基础设施天然契合

### 6.6 建议 6: 持久化 Ancestor 状态

**当前状态:** `last-session.json` 只存储少量元数据（promotion_count 等），不存储完整文件树状态。

**Mutagen 启示:** ancestor 状态持久化是实现增量同步和正确冲突检测的基础。

**改进方向:**
- 扩展 `last-session.json` 或创建独立的 `sync-archive.json`
- 存储上次同步时的文件列表、大小、修改时间
- 用于增量检测和"both modified same"优化

**优先级:** MEDIUM — 需要评估存储格式和兼容性

---

## 7. 关键概念定义

| 概念 | 定义 |
|------|------|
| **Ancestor (Last-Sync)** | 上次同步周期结束时两端达成的一致状态，作为三向合并的基准 |
| **Reconciliation** | 比较 ancestor、alpha、beta 三方状态，生成变更列表和冲突列表的过程 |
| **Staging** | 在应用变更前，将文件临时存放在隔离目录，完成后原子移动到目标位置 |
| **Problematic Entry** | 扫描时遇到错误（如权限拒绝）的文件/目录，被标记为特殊类型，不参与同步 |
| **Untracked Entry** | 被忽略规则排除或无法同步的内容，不参与比较 |
| **Synchronizable** | 经过过滤后实际参与同步比较的内容子集 |
| **TryAgain** | 扫描错误的分类标志，true 表示临时错误应重试，false 表示非临时错误应终止 |
| **Rescan Wait Duration** | 临时扫描错误后的强制等待时间（5秒），避免立即重试循环 |
| **Root Emptied** | 安全机制触发条件：一端删除了根目录内非平凡数量的内容并留下空目录 |
| **Conflict** | 两端对同一内容做了不可自动调和的变更，需要人工干预 |

---

## 8. 来源引用

### 官方文档
- [Mutagen Synchronization Documentation](https://mutagen.io/documentation/synchronization) — 同步模式概述
- [Mutagen Safety Mechanisms](https://mutagen.io/documentation/synchronization/safety-mechanisms) — 安全机制详解
- [Mutagen Staging](https://mutagen.io/documentation/synchronization/staging) — Staging 模式说明
- [Mutagen Probing and Scanning](https://mutagen.io/documentation/synchronization/probing-and-scanning) — 扫描模式说明

### GitHub 源码
- [mutagen-io/mutagen/pkg/synchronization/core/reconcile.go](https://github.com/mutagen-io/mutagen/blob/master/pkg/synchronization/core/reconcile.go) — 三向合并与删除冲突处理
- [mutagen-io/mutagen/pkg/synchronization/core/scan.go](https://github.com/mutagen-io/mutagen/blob/master/pkg/synchronization/core/scan.go) — 扫描逻辑与错误处理
- [mutagen-io/mutagen/pkg/synchronization/core/change.go](https://github.com/mutagen-io/mutagen/blob/master/pkg/synchronization/core/change.go) — 变更类型定义
- [mutagen-io/mutagen/pkg/synchronization/controller.go](https://github.com/mutagen-io/mutagen/blob/master/pkg/synchronization/controller.go) — 状态机与同步循环
- [mutagen-io/mutagen/pkg/synchronization/endpoint.go](https://github.com/mutagen-io/mutagen/blob/master/pkg/synchronization/endpoint.go) — Scan 接口定义
- [mutagen-io/mutagen/pkg/synchronization/core/snapshot.go](https://github.com/mutagen-io/mutagen/blob/master/pkg/synchronization/core/snapshot.go) — 快照结构
- [mutagen-io/mutagen/pkg/synchronization/state.go](https://github.com/mutagen-io/mutagen/blob/master/pkg/synchronization/state.go) — 状态结构

---

*研究完成: 2026-04-30*
*研究员: Claude (gsd-phase-researcher)*
