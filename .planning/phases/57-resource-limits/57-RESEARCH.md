# Phase 57: 资源限制可配置化 - 研究

**研究日期:** 2026-05-29
**领域:** 数据库迁移 / Go API 三态指针 / Docker 资源限制参数 / React 前端表单
**置信度:** HIGH（所有核心文件已逐行审计，源代码与 CONTEXT.md 决策完全对齐）

## 摘要

Phase 57 将当前硬编码的、NOT NULL DEFAULT 的资源限制模型改造为三态可配置模型。当前实现有三级默认值兜底：数据库层（NOT NULL DEFAULT）、UpsertHost 应用层（`defaultIfZero`）、runtime_service 层（`defaultIntIfZero` / `defaultFloatIfZero`）——这些都必须移除，改为由 API 层在创建时（且仅在创建时）填入默认值。数据库列改为 nullable（NULL=无限制），API 使用 Go 指针类型区分"省略=默认 / 0=无限制 / 正值=限制"。新增 PATCH 端点允许在停止状态下修改资源限制，Worker 层补充 `--storage-opt` 磁盘限制参数。前端创建表单当前完全没有资源限制字段，主机详情页也没有资源限制展示和编辑功能，这两个都是本次新增的 UI 能力。

**主要建议:** 先做数据库迁移（0022 migration：ALTER COLUMN DROP NOT NULL），再做 Go 类型变更（int→*int，float64→*float64），然后新增 Repository 更新方法和 API PATCH 端点，Worker 补 DiskLimitGB 字段和 --storage-opt 参数，最后前端新增资源限制选择控件。

## 用户约束（来自 CONTEXT.md）

### 锁定决策

- **D-01:** 数据库列改为 nullable（`ALTER COLUMN DROP NOT NULL`）。NULL=无限制，正值=限制。迁移保留现有值。默认值逻辑移到 API 层（创建时不传资源字段时填入 4096MB / 2.0 CPU / 20GB）。
- **D-02:** API 使用指针类型（`*int` / `*float64`）实现三态：省略=默认 / 0=无限制 / 正值=限制。Create 和 PATCH 一致。
- **D-03:** PATCH 端点 `PATCH /api/admin/hosts/:id/resources` 仅允许 stopped 状态。运行中返回 409。
- **D-04:** 磁盘限制纳入本次实现。Worker `buildCreateArgs()` 新增 `--storage-opt size=Xg`。注意：需要 overlay2 + xfs + pquota，不支持的宿主机 Docker 会忽略该参数。
- **D-05:** 前端创建表单新增资源限制区域（下拉预设+自定义输入）。详情页显示可编辑区域，运行中禁用编辑。
- **D-06:** Worker 层 `HostActionRequest` 新增 `DiskLimitGB`，移除 runtime_service 中的 default 兜底逻辑。

### Claude 自主裁量

- 前端预设值列表可根据宿主机配置调整
- PATCH API 请求验证（最小值、最大值）合理范围
- `--storage-opt` 在目标宿主机不生效时可降级

### 延迟（OUT OF SCOPE）

- 运行中容器热调（`docker update`）
- 用户自助面板资源视图
- 资源使用量监控
- 按角色限制可选范围

## 需求映射

| 需求 ID | 描述 | 研究支撑 |
|---------|------|---------|
| RES-01 | 无限制语义（DB nullable + API 指针三态） | 迁移 0006 现有 NOT NULL 约束，queries.go L362-371 现有 defaultIfZero 逻辑，需全部改造 |
| RES-02 | PATCH API（新增端点 + 指针类型请求体） | admin_hosts.go 现有 Create 模式可复用，router.go L297-314 路由注册模式可复用，UpdateHostMounts L1535-1542 更新模式可复用 |
| RES-03 | 前端控件（预设+自定义选择器） | create-host-dialog.tsx 当前无资源字段，$hostId.tsx 当前无资源展示，需全新构建 |
| RES-04 | 磁盘限制执行（--storage-opt） | worker.go L268-273 现有内存/CPU 参数模式可复用于磁盘，HostActionRequest 当前无 DiskLimitGB |

## 架构责任映射

| 能力 | 主层 | 次层 | 理由 |
|------|------|------|------|
| 资源限制数据持久化 | 数据库/存储 | — | PostgreSQL hosts 表存储权威值 |
| 三态语义转换（指针→DB值） | API/后端 | — | API 层解析指针，转换为 DB 可写值 |
| 默认值注入 | API/后端 | — | 仅在创建时，API 层填入默认值 |
| Docker 资源参数生成 | Worker | — | buildCreateArgs 将 DB 值转为 Docker CLI 参数 |
| 资源限制 UI 展示与编辑 | 浏览器/客户端 | — | React 组件，shadcn/ui Select + Input |

## 标准技术栈

### 核心
| 库 | 版本 | 用途 | 为何标准 |
|----|------|------|---------|
| Go 1.26.1 + net/http | 项目标准 | PATCH API 端点 | 已用于所有现有端点 (router.go) |
| pgx v5.x | 项目标准 | 数据库迁移与查询 | 已用于所有 Repository 方法 |
| PostgreSQL 18.x | 项目标准 | 存储资源限制值 | 项目数据库 |
| React 19.2 + shadcn/ui | 项目标准 | 前端资源选择控件 | 已用于 create-host-dialog.tsx 的 Select 组件 |
| TanStack Router | 项目标准 | 前端路由 | 已用于 $hostId.tsx |

### 辅助
| 库 | 版本 | 用途 | 何时使用 |
|----|------|------|---------|
| `json.NewDecoder` + `json.Marshal` | Go stdlib | 请求/响应序列化 | 所有 API 端点 |
| `*int` / `*float64` Go 指针 | Go stdlib | 三态语义（nil/0/正值） | D-02 明确要求 |

### 无需新增第三方依赖
本次改造所有依赖均为项目已使用的标准库和框架。前端 shadcn/ui 的 Select、Input、Button 组件已就绪。

## 架构模式

### 数据流（创建主机带资源限制）

```
浏览器 (CreateHostDialog)
  │ memory_limit_mb: null | 0 | 4096
  │ cpu_limit: null | 0 | 2.0
  │ disk_limit_gb: null | 0 | 20
  ▼
POST /v1/admin/hosts  (admin_hosts.go Create)
  │ 解析 JSON body → 三态转换
  │   nil → 填入默认值 (4096/2.0/20) → 写入DB
  │   0   → 写入 NULL (无限制)
  │   正值 → 写入该值
  ▼
Repository.UpsertHost()
  │ 写入 hosts 表 (memory_limit_mb=NULL 或 具体值)
  ▼
runtime_service.QueueHostAction()
  │ 构建 HostActionRequest
  │ MemoryLimitMB: 0 或 正值
  │ DiskLimitGB: 0 或 正值 [NEW]
  ▼
worker.buildCreateArgs()
  │ 值 > 0 → --memory Xm / --cpus X.X / --storage-opt size=Xg
  │ 值 <= 0 → 不传参数（Docker 默认无限制）
  ▼
docker create ... → 容器运行
```

### 数据流（PATCH 修改资源限制）

```
浏览器 ($hostId.tsx 详情页)
  │ PATCH /v1/admin/hosts/{hostID}/resources
  │ body: { memory_limit_mb: null | 0 | 4096, ... }
  ▼
admin_hosts.go PatchResources() [NEW]
  │ 检查 host.Status == "stopped" → 否则 409
  │ 三态转换 (同 Create)
  │ 调用 Repository.UpdateHostResources()
  ▼
Repository.UpdateHostResources() [NEW]
  │ UPDATE hosts SET memory_limit_mb=$1, cpu_limit=$2, disk_limit_gb=$3, updated_at=NOW()
  │ WHERE id=$4
  ▼
返回更新后的 Host 对象
  │ 下次 start → worker 使用新值构建 docker create
```

### 推荐项目结构变更

```
internal/store/migrations/
└── 0022_host_resource_limits_nullable.sql  [NEW]

internal/store/repository/
├── models.go           [MODIFY] UpsertHostParams: int→*int, float64→*float64
└── queries.go          [MODIFY] UpsertHost 移除 defaultIfZero，新增 UpdateHostResources

internal/controlplane/http/
├── admin_hosts.go      [MODIFY] Create body 改用指针类型，新增 PatchResources handler
└── router.go           [MODIFY] 注册 PATCH /v1/admin/hosts/{hostID}/resources

internal/agentapi/
└── contracts.go        [MODIFY] HostActionRequest 新增 DiskLimitGB int

internal/runtime/
├── runtime_service.go  [MODIFY] 移除 defaultIntIfZero/defaultFloatIfZero，透传值
└── tasks/worker.go     [MODIFY] buildCreateArgs 新增 --storage-opt，DiskLimitGB 判断

internal/local/
└── local.go            [MODIFY] 注释更新（local 模式不受本次影响，但常量引用需同步）

web/admin/src/
├── components/hosts/
│   ├── create-host-dialog.tsx           [MODIFY] 新增资源限制区域
│   └── resource-limits-selector.tsx     [NEW] 可复用选择器组件
├── hooks/use-hosts.ts                   [MODIFY] 新增 usePatchHostResources hook
└── routes/_dashboard/hosts/$hostId.tsx  [MODIFY] 详情页新增资源限制展示+编辑
```

### 模式 1: API 指针类型三态解析

**内容:** 使用 `*int` 和 `*float64` 区分省略/0/正值
**何时使用:** Create 和 PATCH 请求体解析
**示例:**
```go
// admin_hosts.go Create body
var body struct {
    UserID        string                `json:"user_id"`
    EgressIPID    string                `json:"egress_ip_id"`
    Timezone      string                `json:"timezone"`
    MemoryLimitMB *int                  `json:"memory_limit_mb"`   // nil=默认, 0=无限制, >0=限制
    CPULimit      *float64              `json:"cpu_limit"`          // nil=默认, 0=无限制, >0=限制
    DiskLimitGB   *int                  `json:"disk_limit_gb"`     // nil=默认, 0=无限制, >0=限制
    HostMounts    repository.HostMounts `json:"host_mounts"`
}

// 三态转换函数
func resolveMemoryLimit(mb *int) *int {
    if mb == nil {
        def := 4096
        return &def
    }
    if *mb == 0 {
        return nil  // NULL in DB = unlimited
    }
    return mb
}
```

### 模式 2: Repository 更新方法

**内容:** 参考现有 `UpdateHostMounts` (queries.go L1535-1542) 模式
**何时使用:** PATCH 端点需要持久化资源限制
**示例:**
```go
// queries.go
func (r *Repository) UpdateHostResources(ctx context.Context, hostID string, memoryLimitMB *int, cpuLimit *float64, diskLimitGB *int) error {
    _, err := r.db.Exec(ctx, `
        UPDATE hosts SET memory_limit_mb = $1, cpu_limit = $2, disk_limit_gb = $3, updated_at = NOW()
        WHERE id = $4
    `, memoryLimitMB, cpuLimit, diskLimitGB, hostID)
    return err
}
```

### 模式 3: Worker Docker 参数生成

**内容:** 仅当值 > 0 时传 Docker 参数
**何时使用:** `buildCreateArgs()` 构建 docker create 命令
**当前已实现模式 (worker.go L268-273):**
```go
if request.MemoryLimitMB > 0 {
    args = append(args, "--memory", fmt.Sprintf("%dm", request.MemoryLimitMB))
}
if request.CPULimit > 0 {
    args = append(args, "--cpus", fmt.Sprintf("%.1f", request.CPULimit))
}
// [NEW] DiskLimitGB
if request.DiskLimitGB > 0 {
    args = append(args, "--storage-opt", fmt.Sprintf("size=%dG", request.DiskLimitGB))
}
```

### 应避免的反模式

- **在 Repository 层做默认值兜底:** 当前 `UpsertHost` 中的 `defaultIntIfZero` 逻辑（queries.go L362-371）必须移除。默认值决策应在 API 层统一处理。
- **多层默认值:** 当前有三级默认值（DB DEFAULT → UpsertHost defaultIfZero → runtime_service defaultIntIfZero），每一级都是独立的默认逻辑源，导致行为不一致且无法区分"用户想用默认"和"用户想无限制"。必须收拢到 API 层单一入口。
- **用 0 表示无限制:** 0 在 JSON 中可省略（omitempty），无法区分"省略"和"显式传 0"。必须使用指针类型。

## 不需要手写的轮子

| 问题 | 不要自己造 | 使用 | 原因 |
|------|-----------|------|------|
| JSON 三态解析 | 自定义解析器 | Go `*int` / `*float64` + `json.Decoder` | 标准库 json 包天然支持指针类型：nil=省略，非 nil 零值=0 |
| 前端 Select 组件 | 自定义下拉 | shadcn/ui `Select` + `Input` | 项目中已在 create-host-dialog.tsx 使用，模式成熟 |
| API 路由注册 | 自定义路由 | `net/http` ServeMux pattern `PATCH /path` | Go 1.22+ 原生支持方法路由，router.go 已大量使用 |
| 数据库迁移 | 手工 SQL | 编号 SQL 文件 + pgx 执行 | 项目现有 0021 个迁移文件，模式成熟 |

## 运行时状态清单

> Phase 57 是 greenfield 功能增强，不涉及 rename/refactor。此节省略。

## 常见陷阱

### 陷阱 1: 数据库迁移时 NULL 扫描到 Go int 类型会失败

**问题:** Go 的 `int` / `float64` 不能容纳 SQL NULL 值。当前 models.go 中 `Host.MemoryLimitMB int` 扫描 NOT NULL 列没问题。改为 nullable 后，Scan 会因 NULL 而报错。
**根因:** pgx 对 NULL 扫描到非指针 Go 类型会返回错误。
**预防:** Host struct 的 `MemoryLimitMB`、`CPULimit`、`DiskLimitGB` 必须同步改为 `*int` / `*float64`，所有 Scan 调用点也必须更新。或者使用 `pgx` 的 nullable 类型（如 `pgtype.Int4`），但指针类型更符合 D-02 的三态语义。
**警告信号:** 迁移后 `GetHost` / `ListHosts` 等查询在遇到 NULL 值时 panic 或返回 scan error。
**修复方案（推荐）:** Host 字段改为指针类型。所有扫描点（queries.go 约 8 处 SELECT）同步更新。

### 陷阱 2: 现有主机的默认值在迁移后丢失语义

**问题:** 迁移 `ALTER COLUMN DROP NOT NULL` 后，现有主机的 4096/2.0/20 值仍然存在，但它们代表的是"管理员未明确设置"而非"管理员明确设置了 4096"。
**根因:** 迁移前数据库无法区分"用户选了默认"和"用户没选"。D-01 决定保留现有值不变。
**预防:** 不需要特殊处理。D-01 明确：迁移时保留现有值（已有主机的值是显式限制）。语义上的"这是旧默认值"不影响功能——下次管理员编辑时可以改为无限制。

### 陷阱 3: `json:"omitempty"` 标签与指针 0 值的交互

**问题:** 如果 HostActionRequest 的 `MemoryLimitMB` 使用 `json:"memory_limit_mb,omitempty"` 且类型为 `int`，则 JSON marshal 时 0 会被省略。这会导致 Worker 收不到"无限制"语义。
**根因:** Go 的 omitempty 对 int 类型的 0 值会省略字段。
**预防:** `HostActionRequest` 的 `MemoryLimitMB` 和 `CPULimit` **不要**改为指针类型（Worker 层不需要三态，只需知道"传参数还是不传参数"）。0=无限制在 Worker 层就够用了。API 层的三态转换在 `runtime_service.go` 的 `QueueHostAction` 之前完成。

### 陷阱 4: 前端 HostDetail 接口未包含资源字段

**问题:** 当前 `HostDetail` TypeScript 接口（use-hosts.ts L55-69）没有 `memory_limit_mb`、`cpu_limit`、`disk_limit_gb` 字段。
**根因:** `HostWithUsername` 接口也未包含这些字段，`HostDetail.host` 不包含它们。后端 `Host` struct (models.go) 有这些字段，但前端的 `HostDetail.host` 映射不完整。
**预防:** 需要在 `use-hosts.ts` 的 `HostDetail.host` 接口中新增这三个字段，以及 `HostWithUsername` 接口。后端已通过 API 返回这些字段（GetHost handler 返回完整的 Host 对象），前端只是没有声明类型。

## 代码示例

### 当前实现 → 目标实现对照

#### 1. 数据库迁移

**当前 (0006_host_resource_limits.sql):**
```sql
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS memory_limit_mb INTEGER NOT NULL DEFAULT 4096;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS cpu_limit NUMERIC(4,1) NOT NULL DEFAULT 2.0;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS disk_limit_gb INTEGER NOT NULL DEFAULT 20;
```

**目标 (0022_host_resource_limits_nullable.sql):**
```sql
ALTER TABLE hosts ALTER COLUMN memory_limit_mb DROP NOT NULL;
ALTER TABLE hosts ALTER COLUMN cpu_limit DROP NOT NULL;
ALTER TABLE hosts ALTER COLUMN disk_limit_gb DROP NOT NULL;
-- 移除 DEFAULT 约束（PostgreSQL 中需单独处理）
ALTER TABLE hosts ALTER COLUMN memory_limit_mb DROP DEFAULT;
ALTER TABLE hosts ALTER COLUMN cpu_limit DROP DEFAULT;
ALTER TABLE hosts ALTER COLUMN disk_limit_gb DROP DEFAULT;
```

#### 2. API Create 端点三态转换

**当前 (admin_hosts.go L149-157):**
```go
var body struct {
    UserID        string                `json:"user_id"`
    EgressIPID    string                `json:"egress_ip_id"`
    Timezone      string                `json:"timezone"`
    MemoryLimitMB int                   `json:"memory_limit_mb"`
    CPULimit      float64               `json:"cpu_limit"`
    DiskLimitGB   int                   `json:"disk_limit_gb"`
    HostMounts    repository.HostMounts `json:"host_mounts"`
}
```

**目标:**
```go
var body struct {
    UserID        string                `json:"user_id"`
    EgressIPID    string                `json:"egress_ip_id"`
    Timezone      string                `json:"timezone"`
    MemoryLimitMB *int                  `json:"memory_limit_mb"`   // nil=默认, 0=无限制
    CPULimit      *float64              `json:"cpu_limit"`          // nil=默认, 0=无限制
    DiskLimitGB   *int                  `json:"disk_limit_gb"`     // nil=默认, 0=无限制
    HostMounts    repository.HostMounts `json:"host_mounts"`
}
// ... 之后在传给 UpsertHostParams 之前做三态解析
```

#### 3. HostActionRequest 新增 DiskLimitGB

**当前 (contracts.go L54-55):**
```go
MemoryLimitMB int     `json:"memory_limit_mb,omitempty"`
CPULimit      float64 `json:"cpu_limit,omitempty"`
```

**目标:**
```go
MemoryLimitMB int     `json:"memory_limit_mb,omitempty"`
CPULimit      float64 `json:"cpu_limit,omitempty"`
DiskLimitGB   int     `json:"disk_limit_gb,omitempty"`  // [NEW]
```

#### 4. Worker buildCreateArgs 新增磁盘参数

**当前 (worker.go L268-273):**
```go
if request.MemoryLimitMB > 0 {
    args = append(args, "--memory", fmt.Sprintf("%dm", request.MemoryLimitMB))
}
if request.CPULimit > 0 {
    args = append(args, "--cpus", fmt.Sprintf("%.1f", request.CPULimit))
}
```

**目标（在上述代码块之后追加）:**
```go
if request.DiskLimitGB > 0 {
    args = append(args, "--storage-opt", fmt.Sprintf("size=%dG", request.DiskLimitGB))
}
```

## 当前技术 vs 新技术

| 旧方式 | 新方式 | 变更时机 | 影响 |
|--------|--------|---------|------|
| `int` / `float64` 非指针（无法区分省略/0） | `*int` / `*float64` 指针（三态） | API 层 Create + PATCH | 向后不兼容：调用方须使用 null 而非省略表示默认 |
| 三级默认值兜底（DB/Repo/Service） | API 层单一入口默认值 | UpsertHost + QueueHostAction 移除兜底 | 默认值逻辑收拢，行为可预测 |
| `memory_limit_mb NOT NULL DEFAULT 4096` | nullable，无 DEFAULT | 0022 迁移 | 现有数据保留，新行可为 NULL |
| 无 `--storage-opt` | `--storage-opt size=Xg` | Worker buildCreateArgs | 需 overlay2+xfs+pquota，不支持的宿主机 Docker 会静默忽略 |
| 前端无资源限制 UI | 预设下拉 + 自定义输入 | create-host-dialog + $hostId | 新增 UI 交互，不影响现有功能 |
| 无 PATCH 端点 | `PATCH /v1/admin/hosts/:id/resources` | router.go + admin_hosts.go | 新增 API 端点 |

**已弃用/过时:**
- `defaultIntIfZero` / `defaultFloatIfZero` 函数（runtime_service.go L331-343）：将被移除
- `UpsertHost` 中的 `defaultIfZero` 逻辑（queries.go L362-371）：将被移除
- `HostActionRequest` 的 `MemoryLimitMB` `omitempty` 标签：保留，但语义从"省略=使用默认"变为"0=Worker 层不传参数"

## 假设日志

| # | 假设声明 | 章节 | 风险（如果错误） |
|---|---------|------|-----------------|
| A1 | 现有 Host 的 Scan 全改用指针类型后，pgx 能正确处理 NULL→nil 映射 | 常见陷阱 | pgx 的 Scan 行为可能是将 NULL 扫描为指针类型的 nil（标准行为），需验证 |
| A2 | `--storage-opt size=Xg` 在 overlay2 + xfs (pquota) 宿主机上生效，其他驱动静默忽略 | Worker 层 | 如果 Docker 在某些驱动上拒绝该参数（而非忽略），容器创建会失败 |
| A3 | 前端 `HostDetail.host` 缺少的资源字段从后端 API 实际已返回，只是前端未声明类型 | 常见陷阱 | 需验证 GetHost handler 序列化时是否包含了 MemoryLimitMB 等字段 |

## 待解决问题

1. **Go struct 字段类型从 int 改为 *int 的级联影响范围**
   - 已知: `Host` struct（queries.go 约 8 处扫描）、`UpsertHostParams`、API body struct
   - 未知: 是否有其他代码通过 `host.MemoryLimitMB` 做数值比较或算术运算
   - 建议: 执行 grep 全仓库 `\.MemoryLimitMB\b|\.CPULimit\b|\.DiskLimitGB\b` 确认所有使用点

2. **PATCH 端点请求体验证范围**
   - 内存合理范围: 建议 128MB ~ 256GB
   - CPU 合理范围: 建议 0.1 ~ 宿主机核心数
   - 磁盘合理范围: 建议 1GB ~ 2TB
   - 建议: Planner 确定合理上下限，API 层做区间校验

3. **Docker `--storage-opt size=Xg` 的实际行为**
   - 声称: 仅 overlay2 + xfs 支持，其他驱动静默忽略
   - 需要: 在生产环境（Linux 宿主机）上验证该参数的实际效果
   - 降级策略: 若不支持，可选项为仅记录不强制（D-04 允许）

## 环境可用性

Phase 57 不引入新的外部依赖。所有变更在现有 Go / PostgreSQL / React 技术栈内完成。

| 依赖 | 需要者 | 可用 | 版本 | 备选方案 |
|------|--------|------|------|---------|
| Go | 后端编译 | 是 | 1.26.1 | — |
| PostgreSQL | 数据库迁移 | 是 | 18.x | — |
| Docker Engine | --storage-opt 功能 | 取决于宿主机 | 28.x | 降级为仅记录，不传参数 |
| Node.js | 前端构建 | 是 | 24 LTS | — |

## 安全领域

> `security_enforcement` 未显式禁用（config.json 无此键），默认启用。

### 适用 ASVS 类别

| ASVS 类别 | 适用 | 标准控制 |
|-----------|------|---------|
| V2 认证 | 是 | 现有 JWT adminGuard 中间件 |
| V4 访问控制 | 是 | adminGuard 确保仅管理员可调 PATCH 端点 |
| V5 输入验证 | 是 | PATCH 端点需验证资源限制范围（避免负数、超大值） |

### 已知威胁模式

| 模式 | STRIDE | 标准缓解 |
|------|--------|---------|
| 恶意超大值磁盘限制（如 999999GB） | 拒绝服务 | API 层上限校验（如 2TB） |
| 绕过状态检查修改运行中容器资源 | 权限提升 | PATCH 端点强制检查 `host.Status == "stopped"` |
| 负数资源限制值 | 篡改 | API 层最小值校验（内存 >= 128，CPU >= 0.1） |

## 来源

### 主要来源（HIGH 置信度 — 源代码审计）

- `internal/store/migrations/0006_host_resource_limits.sql` — 当前列定义（NOT NULL DEFAULT）
- `internal/store/repository/models.go` L34-50 — Host struct（int/float64 非指针）
- `internal/store/repository/models.go` L268-281 — UpsertHostParams（int/float64 非指针）
- `internal/store/repository/queries.go` L359-430 — UpsertHost 方法（含 defaultIfZero 逻辑）
- `internal/controlplane/http/admin_hosts.go` L147-282 — Create 端点（body 结构 + 调用链）
- `internal/controlplane/http/router.go` L93-386 — 完整路由注册表（当前无 PATCH 端点）
- `internal/agentapi/contracts.go` L41-73 — HostActionRequest（无 DiskLimitGB）
- `internal/runtime/runtime_service.go` L164-188 — QueueHostAction（含 defaultIntIfZero/defaultFloatIfZero）
- `internal/runtime/tasks/worker.go` L219-340 — buildCreateArgs（内存/CPU 参数，无磁盘参数）
- `internal/local/local.go` L199-237 — local 模式 buildCreateArgs（相同模式）
- `web/admin/src/components/hosts/create-host-dialog.tsx` — 创建表单（无资源字段）
- `web/admin/src/routes/_dashboard/hosts/$hostId.tsx` — 详情页（无资源展示）
- `web/admin/src/hooks/use-hosts.ts` — 前端 API hooks（useCreateHost 无资源字段）

### 次要来源（MEDIUM 置信度 — CONTEXT.md 锁定决策）

- `.planning/phases/57-resource-limits/57-CONTEXT.md` — D-01 至 D-06 锁定决策
- `.planning/phases/57-resource-limits/57-DISCUSSION-LOG.md` — 讨论记录（全部委托给 Claude）

### 未验证来源（LOW 置信度）

- Docker `--storage-opt size=Xg` 参数行为 [ASSUMED: Docker docs] — 需在生产宿主机验证
- pgx NULL Scan 到 *int 类型的行为 [ASSUMED: pgx 文档] — 需单元测试验证

## 元数据

**置信度分解:**
- 标准技术栈: HIGH — 完全基于现有项目技术栈，无新依赖
- 架构: HIGH — 所有变更点通过源代码逐行审计确认
- 陷阱: HIGH — 陷阱 1（NULL Scan 到 int）和陷阱 3（omitempty 交互）基于 Go/pgx 已知行为

**研究日期:** 2026-05-29
**有效期至:** 2026-06-29（30 天，稳定技术栈）
