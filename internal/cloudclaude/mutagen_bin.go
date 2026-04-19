package cloudclaude

import (
	"embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// mutagenFS 嵌入 4 平台 Mutagen v0.18.1 客户端二进制。
// 占位场景下 mutagen_bin/<plat>/mutagen 为 shell stub，运行时 Extract 出来调用 version 会失败，
// 测试用 hasRealMutagenEmbed 跳过依赖真二进制的用例。
//
//go:embed mutagen_bin
var mutagenFS embed.FS

// MutagenBinaryVersion 是 embed 的 Mutagen 客户端版本。
// Plan 02 的版本握手会比对此常量与远端 /etc/cloud-claude/mutagen.version。
const MutagenBinaryVersion = "v0.18.1"

// ExtractMutagenBinary 把当前 GOOS_GOARCH 的 embed mutagen 二进制写到 dst（建议 ~/.cloud-claude/bin/mutagen）。
//
// 行为：
//  1. 父目录不存在则 0700 创建（与 ConfigDir 一致）
//  2. 目标已存在且执行 `<dst> version` 输出含 "0.18.1" → 视为复用，直接 return nil（幂等）
//  3. 否则覆盖写入，权限 0755（先写临时文件再 rename，原子替换）
//  4. 当前平台无对应 embed 二进制 → 返回 errcodes.Format(MOUNT_MUTAGEN_TRANSPORT_FAILED, ...) 包装的 error
func ExtractMutagenBinary(dst string) error {
	return extractMutagenFor(runtime.GOOS+"_"+runtime.GOARCH, dst)
}

// extractMutagenFor 是 ExtractMutagenBinary 的内部 helper，允许测试注入 plat。
func extractMutagenFor(plat, dst string) error {
	switch plat {
	case "darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64":
	default:
		return fmt.Errorf("%s", errcodes.Format(errcodes.MOUNT_MUTAGEN_TRANSPORT_FAILED, "unsupported platform "+plat))
	}

	if isMutagenAtVersion(dst, MutagenBinaryVersion) {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return fmt.Errorf("创建 mutagen 二进制目录失败: %w", err)
	}

	src, err := mutagenFS.Open("mutagen_bin/" + plat + "/mutagen")
	if err != nil {
		return fmt.Errorf("%s", errcodes.Format(errcodes.MOUNT_MUTAGEN_TRANSPORT_FAILED, "embed 缺失 "+plat))
	}
	defer src.Close()

	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("写入 mutagen 临时文件失败: %w", err)
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		os.Remove(tmp)
		return fmt.Errorf("复制 mutagen 二进制失败: %w", err)
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("关闭 mutagen 临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename mutagen 二进制失败: %w", err)
	}
	return nil
}

// isMutagenAtVersion 调用 `<path> version` 输出包含目标版本号（去掉 v 前缀）则视为命中。
// path 不存在或非可执行则返回 false（让上层走覆盖写入流程）。
func isMutagenAtVersion(path, want string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	cmd := exec.Command(path, "version")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), strings.TrimPrefix(want, "v"))
}
