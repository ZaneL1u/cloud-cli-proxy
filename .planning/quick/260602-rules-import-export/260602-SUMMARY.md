---
status: complete
---

# Quick Task 260602: 规则导入导出按钮 - Summary

## 完成内容

- 在主机详情页代理白名单规则区新增「规则导出」和「规则导入」按钮。
- 导出当前规则为本地 JSON 文件，格式包含 `version`、`exported_at`、`rules`。
- 导入支持上传导出文件或直接上传规则数组，并通过现有创建规则 API 追加为自定义规则。
- 导入规则使用 `confirm_risky: true`，成功后提示用户点击「应用」生效。
- 补充按钮展示和 JSON 上传导入的单元测试。

## 修改文件

- `web/admin/src/components/bypass/custom-rules-table.tsx`
- `web/admin/src/components/bypass/__tests__/custom-rules-table.test.tsx`
- `.planning/STATE.md`

## 验证

- `corepack pnpm --dir web/admin typecheck`：未通过运行，原因是 `web/admin/node_modules` 缺失，`tsc` 命令不可用。
- `corepack pnpm --dir web/admin test:unit -- src/components/bypass/__tests__/custom-rules-table.test.tsx`：未通过运行，原因是 `web/admin/node_modules` 缺失，`vitest` 命令不可用。

## 后续

安装前端依赖后重新运行类型检查和相关单元测试。
