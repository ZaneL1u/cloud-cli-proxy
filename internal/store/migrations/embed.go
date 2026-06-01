package migrations

import "embed"

// FS 内嵌当前目录下全部 *.sql 迁移文件。
// 序号文件名 + migrator 按序执行保证迁移一致性。
//
//go:embed *.sql
var FS embed.FS
