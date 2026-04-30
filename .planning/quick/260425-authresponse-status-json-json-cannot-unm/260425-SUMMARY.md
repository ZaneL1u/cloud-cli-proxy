---
phase: quick
plan: "260425"
subsystem: cloud-claude
 tags:
  - bugfix
  - json
  - compatibility
  - status
key-files:
  created: []
  modified:
    - internal/cloudclaude/entry.go
    - internal/cloudclaude/entry_compat_test.go
decisions:
  - Status 类型底层为 string，通过 String() 方法和直接比较运算符保持现有调用点零破坏
  - 数字 status 仅做解析兼容，不改变语义映射（"1" != "ready"），避免误匹配
metrics:
  duration: "15 min"
  completed_date: "2026-04-30"
---

# quick-260425: 修复 AuthResponse.Status 无法解析 JSON 数字类型的问题

**一句话总结：** 为 AuthResponse.Status 引入自定义 `Status` 类型并实现 `json.Unmarshaler`，兼容 JSON 字符串和数字两种形态，彻底消除 `json: cannot unmarshal number into Go struct field AuthResponse.status of type string` 登录报错。

## 变更内容

### Task 1: Status 自定义类型 + json.Unmarshaler 实现

- 新增 `Status` 自定义类型（底层 `string`），实现 `UnmarshalJSON`：
  - 先尝试按字符串解析（带引号）
  - 失败回退到 `int64` 数字解析，再转字符串存储
- `AuthResponse.Status` 字段类型由 `string` 改为 `Status`
- `String()` 方法保持 `resp.Status == "ready"` 等现有比较语法零破坏
- 新增 5 条单元测试：字符串解析、数字解析、数字 0 边界、序列化、比较语法

### Task 2: 端到端测试补充

- `TestAuthenticate_NumberStatus`：httptest 返回 `{"status":1}`，验证 `Authenticate()` 不报错且 SSH 四元组正常读取
- `TestAuthenticateAndWait_NumberStatusPolling`：模拟先返回数字 `0`（未就绪）再返回字符串 `"ready"` 的轮询场景，验证轮询逻辑正确

### Task 3: 全包回归验证

- `go test ./internal/cloudclaude/... -count=1` 全 PASS（含 doctor / errcodes 子包）
- 无回归

## 测试覆盖

| 测试名 | 覆盖场景 |
|--------|----------|
| TestAuthResponse_StatusString | JSON 字符串 `"ready"` 正常解析 |
| TestAuthResponse_StatusNumber | JSON 数字 `1` 解析为 `"1"` |
| TestAuthResponse_StatusNumberZero | JSON 数字 `0` 边界 |
| TestAuthResponse_StatusMarshal | 序列化输出仍为 JSON 字符串 |
| TestAuthResponse_StatusComparison | `==` 直接比较和 `.String()` 均可用 |
| TestAuthenticate_NumberStatus | httptest 端到端数字 status |
| TestAuthenticateAndWait_NumberStatusPolling | 数字→字符串轮询端到端 |

## 偏差记录

无偏差 — 计划按预期执行，未触发 Rule 1-4。

## 已知 Stub

无。所有变更均完整实现，无占位符或 TODO。

## 提交记录

| 任务 | Commit | 说明 |
|------|--------|------|
| Task 1 | `6b1663e` | Status 自定义类型 + UnmarshalJSON + 5 条单元测试 |
| Task 2 | `0aa8766` | 2 条端到端测试（Authenticate + AuthenticateAndWait） |
| Task 3 | `b80861a` | 全包回归验证通过 |

## 自检查验

- [x] `internal/cloudclaude/entry.go` 已修改，包含 Status 类型定义
- [x] `internal/cloudclaude/entry_compat_test.go` 已修改，包含 7 条新测试
- [x] `go build ./internal/cloudclaude/...` PASS
- [x] `go test ./internal/cloudclaude/... -count=1` 全 PASS
- [x] 3 个 commit 均存在于 git 历史

## Self-Check: PASSED
