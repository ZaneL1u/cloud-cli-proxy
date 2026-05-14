---
phase: 51-qual-harden
plan: 51-07
status: completed
completed_at: 2026-05-14
---

# 51-07 SUMMARY — `-race -shuffle=on` 默认

## 落地清单

- `Makefile`：`test-go` target 改为
  `go test $(go list ./... | grep -v '/tests/e2e$') -race -shuffle=on -count=1`
  + `go test ./tests/e2e/... -count=1`（tests/e2e 不带 race，与 CONTEXT 一致）。
- `Makefile`：`ci-gate` target 同步引入 `-race -shuffle=on`。
- `.github/workflows/ci.yml`：`go-test` job 的 `Run Go tests` step 同步改写。

## 验证

- `go test $(go list ./... | grep -v '/tests/e2e$') -race -shuffle=on -count=1`
  全绿（19 个包，最长 internal/cloudclaude 47.7s，其它 < 6s）。
- darwin 上未发现新 race / shuffle 相关 fail。

## 偏差

- 无。
