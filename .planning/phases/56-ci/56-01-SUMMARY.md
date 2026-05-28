---
phase: 56-ci
plan: "01"
requirements-completed: [CI-01, CI-02, CI-03]
subsystem: ci
tags: [ci, makefile, e2e, v4.0]

# Completed tasks
- [x] T1 — CI paths 扩面（+6 路径）
- [x] T2 — Makefile e2e target
- [x] T3 — lint-no-bare-sleep + go vet 串入

# Key changes
- e2e.yml paths: +controlplane/http/**, +store/**, +deploy/docker/**, +Makefile, +go.mod, +go.sum
- Makefile e2e: lint → vet → test, 与 CI lint job 对齐

# Self-Check: PASSED
- [x] grep "controlplane/http" .github/workflows/e2e.yml ✓
- [x] grep "^e2e:" Makefile ✓
- [x] grep "lint-no-bare-sleep" Makefile ✓
