//go:build e2e

package harness

import "context"

// DumpHook 在 e2e harness 中「超时即归档」场景被调用，
// 收集容器日志 / nft ruleset / netns / route / pg_dump 等排障证据。
//
// 当前 Plan 03 仅定义接口与 NoopDumpHook 占位；
// Plan 04 (E2E-04) 提供 artifactDumper 真实实现，并通过
// BaseSuite.TearDownTest 与 WaitFor 的 WithDumpHook 注入。
//
// OnWaitForTimeout 必须是幂等且 best-effort：失败时返回 error 由调用方
// 决定如何呈现，本身不应 panic 或长时间阻塞（建议内部使用 ctx 配合短
// 超时，避免 e2e 在 dump 阶段卡死整个 suite）。
type DumpHook interface {
	OnWaitForTimeout(ctx context.Context, name string, lastErr error) error
}

// NoopDumpHook 是默认实现，不做任何动作。
// WaitFor 在没有显式 WithDumpHook 时使用它，让 Plan 03 自己的 unit test
// 可以不依赖 docker / 真实容器跑通。
type NoopDumpHook struct{}

// OnWaitForTimeout 直接返回 nil。
func (NoopDumpHook) OnWaitForTimeout(_ context.Context, _ string, _ error) error {
	return nil
}

// 静态断言 ArtifactDumper（Plan 04）实现 DumpHook 接口。
// 若 ArtifactDumper.OnWaitForTimeout 签名漂移，编译期立即失败。
var _ DumpHook = (*ArtifactDumper)(nil)
