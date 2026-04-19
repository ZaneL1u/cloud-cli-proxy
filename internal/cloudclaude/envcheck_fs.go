package cloudclaude

import (
	"os"
	"path/filepath"
	"strings"
)

// IsCaseInsensitiveFS 跨平台 probe 当前路径所在文件系统是否大小写不敏感
// （macOS APFS 默认 / Windows NTFS 默认）。
//
// 实现：在 dir 下创建一个含小写名的临时文件，Stat 其全大写变体；
//
//	Stat 成功 → 文件系统不区分大小写。
//
// 失败（无写权限、临时目录不可用、文件名本身已全大写）→ 返回 false（保守降级，避免 panic）。
//
// 不依赖 macOS diskutil，因此在容器化 / sandbox 环境也可用（替代 D-09 早期方案）。
func IsCaseInsensitiveFS(dir string) bool {
	f, err := os.CreateTemp(dir, "ccprobe-")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	defer os.Remove(name)

	upper := filepath.Join(filepath.Dir(name), strings.ToUpper(filepath.Base(name)))
	if upper == name {
		return false
	}
	if _, err := os.Stat(upper); err == nil {
		return true
	}
	return false
}
