//go:build e2e

// Package harness 提供 Cloud CLI Proxy e2e 套件的可复用基础设施。
//
// 当前包含 BaseSuite（生命周期 hook + 上下文 + 日志器 + 项目根定位）。
// 后续 plan 会陆续补充 Scenario builder（Plan 02）、waitFor helper（Plan 03）、
// artifact dump（Plan 04）。
package harness

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/stretchr/testify/suite"
)

// BaseSuite 是所有 e2e suite 的基础。它提供：
//
//   - Ctx / Cancel：suite 生命周期上下文，SetupSuite 创建、TearDownSuite 取消。
//   - Logger：结构化日志器，key-value 风格与项目主代码（log/slog）保持一致。
//   - ProjectRoot：仓库根目录（基于 runtime.Caller 反推），便于 fixture 文件按
//     项目相对路径定位；禁止任何代码硬编码 /Users/... 这类绝对路径。
//
// 子 suite 通过结构体嵌入 *BaseSuite 复用以上字段，并按需在自己的
// SetupSuite/TearDownSuite 中先调用基类版本：
//
//	func (s *MySuite) SetupSuite() {
//	    s.BaseSuite = &harness.BaseSuite{}
//	    s.BaseSuite.SetT(s.T())
//	    s.BaseSuite.SetupSuite()
//	}
//	func (s *MySuite) TearDownSuite() { s.BaseSuite.TearDownSuite() }
type BaseSuite struct {
	suite.Suite

	Ctx         context.Context
	Cancel      context.CancelFunc
	Logger      *slog.Logger
	ProjectRoot string

	// dumper 由调用方在 SetupSuite / SetupTest 中通过 SetArtifactDumper 注入。
	// 未注入时 TearDownTest 失败分支只 logger.Warn，不阻塞用例。
	// Plan 04 引入；Phase 52 OBS-01..03 接入完整收集逻辑后行为不变。
	dumper *ArtifactDumper
}

// SetArtifactDumper 在 SetupSuite 或 SetupTest 阶段由调用方注入。
// Plan 02 Scenario 落地后，典型用法：
//
//	s.SetArtifactDumper(harness.NewArtifactDumper(scenario, ""))
func (s *BaseSuite) SetArtifactDumper(d *ArtifactDumper) { s.dumper = d }

// ArtifactDumper 返回当前注入的 dumper；未注入时返回 nil。
func (s *BaseSuite) ArtifactDumper() *ArtifactDumper { return s.dumper }

// SetupSuite 在整个 suite 跑第一个用例之前执行一次。
func (s *BaseSuite) SetupSuite() {
	s.Ctx, s.Cancel = context.WithCancel(context.Background())
	s.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	s.ProjectRoot = projectRootFromCaller()
	s.Logger.Info("e2e BaseSuite ready", "project_root", s.ProjectRoot)
}

// TearDownSuite 在整个 suite 跑完最后一个用例之后执行一次。
func (s *BaseSuite) TearDownSuite() {
	if s.Cancel != nil {
		s.Cancel()
	}
}

// SetupTest 留作 Plan 02+ 的 Scenario 注入点（当前为空）。
func (s *BaseSuite) SetupTest() {}

// TearDownTest 在每个 Test* 用例结束后被 testify/suite 自动调用。
//
// Plan 04 起的语义：若 t.Failed() 为 true 且 dumper 已通过 SetArtifactDumper
// 注入，自动调用 dumper.Collect(s.Ctx, s.T().Name()) 把排障证据归档；
// 用例成功时不动 disk，避免污染 artifact 目录。
//
// dumper 未注入时仅 logger.Warn，不阻塞用例（适用于无 artifact 需求的场景）。
func (s *BaseSuite) TearDownTest() {
	if !s.T().Failed() {
		return
	}
	if s.dumper == nil {
		s.Logger.Warn("test failed but no artifact dumper configured", "test", s.T().Name())
		return
	}
	dir, err := s.dumper.Collect(s.Ctx, s.T().Name())
	if err != nil {
		s.Logger.Warn("artifact collect failed",
			"test", s.T().Name(), "err", err)
		return
	}
	s.Logger.Info("artifact collected on failure",
		"test", s.T().Name(), "dir", dir)
}

// projectRootFromCaller 通过 runtime.Caller 反推仓库根目录（go.mod 所在目录）。
// 不依赖 git，也不依赖 CWD（go test 在不同目录下 CWD 不稳定）。
func projectRootFromCaller() string {
	_, file, _, _ := runtime.Caller(0) // tests/e2e/harness/suite.go
	// 向上 3 级回到仓库根：harness/ → e2e/ → tests/ → <root>
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
