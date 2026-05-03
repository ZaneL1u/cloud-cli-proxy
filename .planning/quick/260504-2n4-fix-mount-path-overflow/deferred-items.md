# Deferred Items — Quick 260504-2n4

执行 quick task 260504-2n4 时发现的、与本次任务无关的既有 TS 类型错误（out-of-scope）。

## 既有 typecheck 报错（与本任务无关）

`pnpm run typecheck` 在以下既有文件中报错，均与挂载路径溢出修复无关，且在本任务改动前即已存在：

| 文件 | 行 | 错误 |
|------|----|------|
| `web/admin/src/components/egress-ips/egress-ip-drawer.tsx` | 183 / 293 | `Resolver` / `SubmitHandler` 泛型不匹配（react-hook-form + zod resolver 类型不一致） |
| `web/admin/src/hooks/use-hosts.ts` | 390 | `Cannot find name 'toast'`（缺少 `toast` 导入） |
| `web/admin/src/lib/api.ts` | 7 | TS1294 erasableSyntaxOnly 限制下的语法 |
| `web/admin/src/routes/_dashboard/hosts/index.tsx` | 81 / 82 | 解构属性 `hosts` / `tasks` 类型未对齐 |

本次仅修改 `create-host-dialog.tsx` 与 `mount-manager.tsx`，两个文件均未新增 TS 报错。
后续可单独立 quick task 修复以上既有错误。
