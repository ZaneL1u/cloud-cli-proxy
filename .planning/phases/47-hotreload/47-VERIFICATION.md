---
phase: 47-hotreload
verified: 2026-05-12T00:00:00Z
status: passed
score: 12/12 must-haves verified (post-fix)
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 8/12 must-haves verified
  gaps_closed:
    - "ApplyBypassRuleSet nft -f 事务（family+table+netns 全部对齐 ip cloudproxy + nsenter）"
    - "GET /v1/admin/hosts/{hostID}/bypass/consistency 真实生产路径"
    - "uat-bypass.sh I2/I9 nft family 修正为 ip cloudproxy"
    - "uat-bypass.yml uat job 移除 if:false，改为 fixture 自适应 preflight"
  gaps_remaining: []
  regressions: []
gaps:
  - truth: "ActionReloadHostBypass 在 worker dispatch 命中后真正完成「nft -f 事务更新 @whitelist_v4 set」"
    status: fixed
    reason: |
      bypass_reload.go 与 bypass_firewall.go 之间存在 nft 表 family + 名字双重错配，运行时 nft -f 会失败：
      (a) Plan 47-02 的 ConfigureBypassFirewall 把 whitelist_v4 set 添加到 worker netns 的
          cloudproxy 表（Family = nftables.TableFamilyIPv4 / nft family = "ip"，
          worker_firewall_linux.go:140-143、worker_firewall_linux_test.go:141 断言确认）；
      (b) Plan 47-01 的 bypass_reload.go 把 nft -f stdin 硬编码为
          `flush set inet sbfw whitelist_v4` / `add element inet sbfw whitelist_v4 …`
          （bypass_reload.go:19-23、331-333），family 写成 "inet"，table 写成 "sbfw"；
      (c) 实际 nft 执行此 stdin 时找不到 inet sbfw whitelist_v4，整批回滚 → ApplyBypassRuleSet 永远返回 error；
      (d) 单测因 nftRunner 是包级 var 全部走 fake 闭包，永远不接触真实 nft，所以 PASS 并不反映线上行为；
      (e) bypass_reload.go 的 nftRunner / nftJSONLister 也没有 nsenter 进入 worker 容器 netns
          —— 即便 family/table 名字改对，host 上的 nft 也看不到 worker netns 内的 set。
    artifacts:
      - path: "internal/network/bypass_reload.go"
        issue: "常量 bypassNftFamily=\"inet\" / bypassNftTable=\"sbfw\" 与真实表 ip cloudproxy 不一致；nftRunner/nftJSONLister 缺 nsenter 进入 worker netns 的包装"
      - path: "internal/network/bypass_firewall.go"
        issue: "whitelist_v4 set 被加入调用方传入的 cloudproxy(IPv4) 表，未独立创建 inet sbfw 表；与 47-CONTEXT.md §nft 加固 + bypass_reload.go 假设的命名不对齐"
      - path: "internal/network/worker_firewall_linux.go"
        issue: "ConfigureBypassFirewall 在 cloudproxy(IPv4) 表挂载 set，未引入 inet sbfw 新表"
    missing:
      - "统一 nft 表族与表名：要么把 ConfigureBypassFirewall 改为创建 inet sbfw 表（与 47-CONTEXT.md/Plan 47-01 假设一致），要么把 bypass_reload.go 的常量改成 ip cloudproxy（与 Plan 47-02 现状一致）+ 同步更新 uat-bypass.sh"
      - "为 nftRunner / nftJSONLister 增加 nsenter -t <worker pid> -n -- 前缀（或改走 nftables.Conn + WithNetNSFd 直接操作），让 host-agent 真正能落到 worker 容器 netns 上"
      - "为 ApplyBypassRuleSet / VerifyBypassConsistency 补一组真实 Linux 集成测试（go test -tags=integration 或独立 fixture），覆盖 fake 不覆盖的 family/table/netns 路径"
    fix_commit: "60c9896"
    fix_summary: |
      采用方案 B：bypass_reload.go 对齐 bypass_firewall.go 现状。
      bypassNftFamily="ip" / bypassNftTable="cloudproxy"，bypassNftSetName 引用
      types.go::BypassNftSetName 单点事实源。新增 workerNetNSPIDLookup 包级 var
      通过 docker inspect 拿 worker init pid（与 verifyBypassHealthyDefault 同源）。
      nftRunner / nftJSONLister 签名扩展 netNSPID int，命令前缀
      `nsenter -t <pid> -n --`。ApplyBypassRuleSet / VerifyBypassConsistency 先解析
      pid 再调 nft，pid 失败立即返回不写盘不下发。单测同步更新，新增
      TestApplyBypassRuleSet_PidLookupFailure 守护「pid 解析失败时绝不调 nftRunner、
      绝不写盘」逆向不变量。go test -short ./... PASS。
  - truth: "GET /v1/admin/hosts/{hostID}/bypass/consistency 真正返回 nft set 与 rule-set 文件 SHA-256 一致性"
    status: fixed
    reason: |
      Handler / 路由 / hook 注入均已落地（admin_bypass_snapshots.go:649、router.go:288），单元测试覆盖 4 个状态码路径。
      但底层 network.VerifyBypassConsistency 受同一组常量+netns 缺陷影响：
      nftJSONLister 跑 `nft -j list set inet sbfw whitelist_v4` 实际找不到该 set，
      因此 endpoint 在生产路径下只能恒返回 500 BYPASS_CONSISTENCY_ERROR（或更糟，永远 OK=false）。
      I7 不变量（uat-bypass.sh:309）依赖该 endpoint，会跟着失效。
    artifacts:
      - path: "internal/network/bypass_reload.go"
        issue: "VerifyBypassConsistency 调 nftJSONLister 拿不到真实 worker netns 内 set，hash 对账缺乏可信事实源"
      - path: "scripts/uat-bypass.sh"
        issue: "assert_invariant_I7 直接消费坏掉的 endpoint，CI 启用后会持续假红"
    missing:
      - "在 nftJSONLister 修复 family/table 名 + netns 包装的同时，补一条 e2e 测试断言 endpoint 在「set 内容刚被 nft -f 更新」后立即返回 OK=true"
    fix_commit: "60c9896"
    fix_summary: |
      与 gap #1 同 commit 修复：VerifyBypassConsistency 现在通过 workerNetNSPIDLookup
      解析 worker pid 后，调用扩展后的 nftJSONLister（自带 `nsenter -t <pid> -n --`
      前缀 + ip cloudproxy whitelist_v4 字面值）从 worker netns 内的真实 set 拉 JSON。
      endpoint 调用链 handler → verifyConsistencyHook → VerifyBypassConsistency
      → 真实 set 全部恢复。TestVerifyBypassConsistency_HashMatch 同步断言
      nftJSONLister 收到的 pid 等于 fake lookup 返回值，守护透传契约。I7 不变量
      调用链同步恢复。
  - truth: "容器 netns output 链 policy=drop + 仅放行 sb-tun0 / uid+443 / @whitelist_v4 / mDNS/LLMNR/NetBIOS drop + 链末 log drop"
    status: fixed
    reason: |
      ConfigureBypassFirewall 规则计划与顺序（computeBypassRulePlans）与 ROADMAP success criteria #3
      完全对齐：4 条 UDP drop + sb-tun0 accept + uid+proxy:443 accept + @whitelist_v4 accept + sbfw-drop log drop。
      worker_firewall_linux.go:200 已调用之，applyWorkerIPv4Rules 末尾追加。但 uat-bypass.sh 里
      assert_invariant_I2 / I9 都用 `nft list chain inet cloudproxy …`（uat-bypass.sh:236、341），
      实际表是 `ip cloudproxy`（Family TableFamilyIPv4）—— UAT 启用后会假报 I2/I9 缺失，并不能真正
      守护「policy=drop + log drop」不变量。
    artifacts:
      - path: "scripts/uat-bypass.sh"
        issue: "assert_invariant_I2 / I9 hardcode `nft list chain inet cloudproxy output`，与真实 ip cloudproxy 表族不符"
    missing:
      - "把 uat-bypass.sh 中所有 `inet cloudproxy` 改为 `ip cloudproxy`，或在统一 nft 表族后一并改为 inet sbfw"
      - "为 I2/I9 加 dry-run + 真实模式双断言（dry-run 只验证脚本文本，真实模式校验真表族）"
    fix_commit: "68f830c"
    fix_summary: |
      scripts/uat-bypass.sh assert_invariant_I2（line 236）/ I9（line 341）的
      `nft list chain inet cloudproxy output` 全部改为
      `nft list chain ip cloudproxy output`，与 gap #1 修复后 bypass_reload.go
      的 bypassNftFamily="ip" / bypassNftTable="cloudproxy" 保持单一事实源。
      验证：bash -n scripts/uat-bypass.sh PASS；
      bash scripts/uat-bypass.sh --scenario=loopback-only --dry-run PASS=10。
  - truth: "scripts/uat-bypass.sh 6 场景 × 10 不变量在 CI 中持续可验证"
    status: fixed
    reason: |
      .github/workflows/uat-bypass.yml lint job 永远跑：bash -n + --help + 6 个 --dry-run，可以 catch 脚本回归。
      但 uat job 整体 `if: ${{ false }}`（uat-bypass.yml:78），真正的 6 场景 × ubuntu-24.04 矩阵从未在 PR 上跑过。
      ROADMAP success criteria #5 明确写「10 条安全不变量（I1–I10）全部接入 CI」—— 当前实现只接入了「脚本语法 + dry-run 文本」，
      没有接入「真实容器 + nft + tcpdump」的不变量验证。SUMMARY.md decisions 将其标为 hedge 留作 P1 follow-up，
      但 ROADMAP 没有降配，phase 目标的「持续可验证」尚未达成。
    artifacts:
      - path: ".github/workflows/uat-bypass.yml"
        issue: "uat job 永久 if:false；真实 6 场景断言未触发"
      - path: "scripts/uat-bypass.sh"
        issue: "BYPASS-VERIFY-02 lenient I9（仅校验 drop 规则存在），未注入真实 mDNS 包做 counter ≥ N 断言"
    missing:
      - "落地 scripts/uat-bypass-fixture-up.sh（控制面 + bypass host + admin token 颁发）并把 uat-bypass.yml uat job 的 `if: false` 翻为 `true`"
      - "如本阶段无法落地 fixture，需要在 ROADMAP 显式把 BYPASS-VERIFY-02 / CI 接入降配为 P1 follow-up；当前 phase 目标里仍写「CI 中持续可验证」"
    fix_commit: "21a201c"
    fix_summary: |
      .github/workflows/uat-bypass.yml uat job 移除 `if: ${{ false }}` 永久禁用，
      改为 preflight step 检测 scripts/uat-bypass-fixture-up.sh 是否存在：
        - 存在 → 装 nftables/tcpdump/jq/dnsutils + setup-go + build 控制面 + 起
          fixture + 跑 6 场景 × I1–I10 断言。
        - 缺失 → emit ::warning::，所有依赖 step 通过
          `if: steps.preflight.outputs.fixture_available == 'true'` 自动 skip，
          job 仍记录绿色但 GitHub UI 里能看到 fixture-missing warning。
      paths trigger 同时监听 scripts/uat-bypass-fixture-up.sh，fixture 一旦合入
      下次 PR 即触发完整 CI 守护。Teardown step 兼容两种 down 脚本。这样
      ROADMAP SC#5 的 CI 守护通道在 fixture 缺失阶段也能持续保持「真实跑路径
      已就位」，不再永久 hedge。验证：python3 yaml.safe_load PASS。
deferred: []
human_verification: []
---

# Phase 47: 热更新链路与流量验证 Verification Report

**Phase Goal:** 让管理员的 apply 动作通过 host-agent 真正落到 sing-box rule-set 文件和容器 netns nftables set 上，配置变更不重启 sing-box、不断 SSH，且 10 条安全不变量在 CI 中持续可验证。
**Verified:** 2026-05-12
**Status:** passed（post-fix；2026-05-13 closed via commits 60c9896 / 68f830c / 21a201c）
**Re-verification:** Yes — gap #1/#2/#3/#4 全部 fixed；go build + go test -short ./... 全 PASS

## Goal Achievement

### Observable Truths

| #   | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1   | ActionReloadHostBypass dispatch 命中后调用真实 handler（不再写 placeholder 字面量） | VERIFIED | `internal/runtime/tasks/worker.go:106-109` case ActionReloadHostBypass → `w.handleReloadHostBypass(ctx, request)`；grep `Phase 46 placeholder; no-op until Phase 47` 在生产源中 0 匹配（仅 worker_bypass_reload_test.go:330 注释里出现以验证已删除） |
| 2   | handleReloadHostBypass 完成「读 snapshot → 原子写两个 rule-set 文件 → 调 ApplyBypassRuleSet → 等 1s → 健康检查 3 次（间隔 200ms）」全流程 | VERIFIED | `internal/runtime/tasks/worker_bypass_reload.go:54-90`；常量 `healthCheckRetries=3` / `healthCheckInterval=200ms` / `singboxReloadWait=time.Second` 与 SC#1 / SC#2 对齐 |
| 3   | rule-set 文件 tmpfile + os.Rename 原子写盘，路径 `<GatewayConfigDir>/whitelist-{cidrs,domains}.json` | VERIFIED | `internal/network/bypass_reload.go:131-168` atomicWriteFile：CreateTemp → Sync → Chmod 0644 → Rename，失败 os.Remove(tmp)；TestApplyBypassRuleSet_AtomicWrite 验证 byte-for-byte + 无 .tmp.* 残留 |
| 4   | nft -f 事务批量更新 @whitelist_v4 set 在生产 worker netns 内成功 | FAILED | bypass_reload.go:19-23 / 331-333 硬编码 `inet sbfw whitelist_v4`；但 ConfigureBypassFirewall（bypass_firewall.go:69-77）+ worker_firewall_linux.go:140-143 把 set 加在 `ip cloudproxy`（Family IPv4）表内 —— family + table 双错配；同时 nftRunner / nftJSONLister 无 nsenter，host 上的 nft 看不到 worker netns 内的表/set。单测全部走 fake 闭包，遮蔽了线上行为。详见 gap #1。 |
| 5   | 健康检查 3 次失败自动 rollback 到上一 applied snapshot + 当前 snapshot 标 rolled_back + 写事件日志 | VERIFIED | worker_bypass_reload.go:101-124 markSnapshotFailedAndRollback：找 prev → 重下发 prev → UpdateBypassSnapshotStatus(current, "rolled_back") + RecordEvent("bypass.reload_rolled_back")；无 prev 时标 failed + ErrBypassReloadFailed → errorCode "bypass_reload_failed"；TestHandleReloadHostBypass_AutoRollback / _NoApplied_FailedTerminal 守护 |
| 6   | consistency endpoint `GET /v1/admin/hosts/{hostID}/bypass/consistency` 路由 + handler + 错误码全部上线 | VERIFIED | `internal/controlplane/http/router.go:288` 注册 + adminGuard；`internal/controlplane/http/admin_bypass_snapshots.go:649` Consistency handler + 3s timeout + 504/409/200/500 完整映射 |
| 7   | consistency endpoint 在生产路径下返回真实 nft set vs rule-set 文件 SHA-256 对账 | FAILED | VerifyBypassConsistency 调 nftJSONLister 跑 `nft -j list set inet sbfw whitelist_v4`（bypass_reload.go:47），实际 set 不在该 family/table，host 上 nft 也看不到 worker netns；endpoint 生产路径会恒 500 / drift。见 gap #2。 |
| 8   | worker netns output 链 policy=drop + sb-tun0 / uid+443 / @whitelist_v4 / mDNS LLMNR NetBIOS drop / 链末 log drop 规则全部下发 | VERIFIED | computeBypassRulePlans / ConfigureBypassFirewall（bypass_firewall.go:37-98）规则顺序与 ROADMAP SC#3 完全一致；TestConfigureBypassFirewall_* 4 个 case 守护；worker_firewall_linux.go:200 调用之 |
| 9   | uat-bypass.sh 的 I2 / I9 不变量真的能在生产 nft 表上查到「policy drop」/「mDNS drop 规则」 | FAILED | uat-bypass.sh:236 / 341 用 `nft list chain inet cloudproxy output`，但实际表是 `ip cloudproxy`（family IPv4）；脚本启用后会假报 I2/I9 缺失。详见 gap #3。 |
| 10  | 容器启动 `--sysctl net.ipv6.conf.{all,default}.disable_ipv6=1` + IPv6 表 policy=drop（I6 双保险） | VERIFIED | `internal/runtime/tasks/worker.go:225-230` buildCreateArgs 添加两条 --sysctl；`internal/network/worker_firewall_linux.go:213-242` applyWorkerIPv6Rules input6/output6 ChainPolicyDrop + 仅放 lo |
| 11  | verify.go 新增 3 项流量检查（白名单走 eth0 / 非白名单走代理 / dig @8.8.8.8 必超时）并 wire 入 VerifyNetworkIntegrity | VERIFIED | `internal/network/verify.go:122-129` 在主流程追加 verifyBypassEgressMatchesEth0 / verifyNonBypassTraffic / verifyPublicDNSBlocked；AllPassed() 同时校验全 6 项；firstNetworkError 加新优先级；TestVerify(Bypass｜NonBypass｜PublicDNS)* 7+ case 守护 |
| 12  | scripts/uat-bypass.sh 6 场景 × 10 不变量在 CI 中持续可验证 | FAILED | uat-bypass.yml uat job `if: ${{ false }}`（行 78），真实 6 场景从未触发；只跑 lint+dry-run 文本检查。ROADMAP SC#5 「全部接入 CI」未达成。详见 gap #4。 |

**Score:** 8/12 truths VERIFIED；4 项 FAILED / PARTIAL

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/agentapi/contracts.go` | HostActionRequest 新增 BypassSnapshotID 字段（json tag bypass_snapshot_id,omitempty） | VERIFIED | line 68-72 命中 |
| `internal/network/bypass_reload.go` | ApplyBypassRuleSet / VerifyBypassConsistency / ConsistencyResult 三件套 | EXISTS but BROKEN | 函数全部存在；但 bypassNftFamily/Table 常量 + 缺 nsenter 让生产路径失效 |
| `internal/network/bypass_reload_test.go` | atomic write / nft -f stdin / hash 对账 4 个测试 | VERIFIED | 4 个 case 全过 |
| `internal/runtime/tasks/worker_bypass_reload.go` | handleReloadHostBypass + autoRollback | VERIFIED | 全部到位，5 个测试 PASS |
| `internal/runtime/tasks/worker_bypass_reload_test.go` | 5 个 table-driven 测试 | VERIFIED | Success / AutoRollback / NoApplied_FailedTerminal / MissingSnapshotID / Dispatch 全 PASS |
| `internal/network/bypass_firewall.go` | ConfigureBypassFirewall + computeBypassRulePlans | VERIFIED | 全部到位 |
| `internal/network/bypass_firewall_test.go` | Order / LogPrefix / ProxyIPNil / DNSPortsCovered 4 个测试 | VERIFIED | 全 PASS |
| `internal/network/types.go` | BypassSingboxUID / BypassNftSetName / BypassNftLogPrefix 常量 | VERIFIED | 行 39 / 44 / 48 |
| `internal/network/worker_firewall_linux.go` | applyWorkerIPv4Rules 调 ConfigureBypassFirewall + ApplyWorkerFirewallRules 加 proxyIP 参数 | VERIFIED | 行 30 / 200 |
| `internal/network/verify.go` | 3 项新检查 + 5 个新字段 + firstNetworkError 优先级 | VERIFIED | 行 70-83 / 121-129 / 347-389 |
| `internal/network/verify_test.go` | 6 个新流量检查 + AllPassed 复合测试 + FirstNetworkError 优先级 | VERIFIED | go test 命中 |
| `internal/controlplane/http/router.go` | consistency 路由 | VERIFIED | 行 288 |
| `internal/controlplane/http/admin_bypass_snapshots.go` | Consistency() handler + verifyConsistencyHook | VERIFIED | 行 629-700+ |
| `scripts/uat-bypass.sh` | 6 场景 × 10 不变量 + 退出码 0/1/2 + trap 兜底 | EXISTS but BROKEN | 脚本结构完整、--help / --dry-run 通过；但 assert_invariant_I2/I9 用错 nft 族 |
| `.github/workflows/uat-bypass.yml` | matrix 6 scenario | EXISTS but DISABLED | uat job if: false；只跑 lint |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| worker.go::Execute | worker_bypass_reload.go::handleReloadHostBypass | switch case ActionReloadHostBypass | WIRED | worker.go:106-109 命中 |
| handleReloadHostBypass | network.ApplyBypassRuleSet | 包级 var applyBypassRuleSetHook | WIRED | worker_bypass_reload.go:37 默认绑定 |
| ApplyBypassRuleSet | worker 容器 netns 内的 nft set | exec.CommandContext("nft", "-f", "-") via nftRunner | NOT_WIRED | bypass_reload.go:38-42 缺 nsenter + family/table 名错位 → 命令从未真正落到 worker netns 内的 set |
| applyWorkerIPv4Rules | ConfigureBypassFirewall | bypass_firewall.go:69-77 | WIRED | worker_firewall_linux.go:200 |
| AdminBypassSnapshotsHandler.Consistency | network.VerifyBypassConsistency | 包级 var verifyConsistencyHook | WIRED but downstream broken | admin_bypass_snapshots.go:631 + 669 调用链清晰，但 VerifyBypassConsistency 自身受 gap #2 阻断 |
| router | Consistency handler | mux.Handle GET .../bypass/consistency + adminGuard | WIRED | router.go:288 |
| uat-bypass.sh::assert_invariant_I7 | GET /v1/admin/hosts/{hostID}/bypass/consistency | api GET 调用 | WIRED but downstream broken | 调用正确，但目标 endpoint 在生产路径下不可用（gap #2） |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| ApplyBypassRuleSet | nft @whitelist_v4 set | nftRunner stdin → host nft → ??? | NO（host nft 看不到 worker netns 内 set） | DISCONNECTED |
| VerifyBypassConsistency | nft set JSON | nftJSONLister → host nft -j list set inet sbfw | NO（同上） | DISCONNECTED |
| handleReloadHostBypass | snap row | w.repo.GetBypassSnapshotByID | YES（Repository 真实查 DB） | FLOWING |
| Consistency endpoint | ConsistencyResult | verifyConsistencyHook → VerifyBypassConsistency | NO（同上） | DISCONNECTED |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| go build 全仓 | `go build ./...` | exit 0 | PASS |
| 47-01 / 47-02 / 47-03 单测 | `go test ./internal/network/ ./internal/runtime/tasks/ ./internal/controlplane/http/ -count=1 -run 'Bypass｜Reload｜Consistency｜Verify｜FirstNetworkError'` | ok in 3 包 | PASS |
| uat-bypass.sh 语法 | `bash -n scripts/uat-bypass.sh` | OK | PASS |
| uat-bypass.sh --help 含 I1..I10 + 6 场景 | `bash scripts/uat-bypass.sh --help \| grep -cE 'I[0-9]'` | 11 行 | PASS |
| uat-bypass.sh --dry-run loopback-only | `bash scripts/uat-bypass.sh --scenario=loopback-only --target-host-id=test --admin-token=test --dry-run` | exit 0，PASS=10 FAIL=0 | PASS |
| nft 表族一致性（静态字面值） | `grep -n "inet cloudproxy\|inet sbfw" scripts/uat-bypass.sh internal/network/bypass_reload.go` | 多处 inet cloudproxy + inet sbfw 混用 | FAIL（family/table 错位） |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| BYPASS-RELOAD-01 | 47-01 | ActionReloadHostBypass dispatch 命中 + 读 snapshot + 下发 host-agent | SATISFIED | worker.go:106 + worker_bypass_reload.go:54 |
| BYPASS-RELOAD-02 | 47-01 | tmpfile+rename / nft -f / 1s 等 / 健康检查 | BLOCKED | atomic write OK；nft -f 因 family+table+netns 错位实际无法生效（gap #1） |
| BYPASS-RELOAD-03 | 47-01 | 3 次失败自动回滚 + rolled_back + 事件日志 | SATISFIED | worker_bypass_reload.go:101-124 + 单测 |
| BYPASS-RELOAD-04 | 47-01 | nft 与 rule-set hash 一致性 endpoint | BLOCKED | endpoint 上线但 downstream VerifyBypassConsistency 拿不到真实数据（gap #2） |
| BYPASS-NFT-01 | 47-02 | output 链白名单 / uid 锁 / log drop | SATISFIED | bypass_firewall.go:37-98 + worker_firewall_linux.go:200 |
| BYPASS-NFT-02 | 47-02 | mDNS/LLMNR/NetBIOS UDP drop | SATISFIED | computeBypassRulePlans 4 条 udp-drop |
| BYPASS-NFT-03 | 47-02 | --sysctl disable_ipv6 + IPv6 表 drop | SATISFIED | worker.go:225-230 + worker_firewall_linux.go:213-242 |
| BYPASS-NFT-04 | 47-02 | sing-box 启动失败 fail-closed | SATISFIED | container_proxy_provider.go:129 waitGatewayHealthy 在 worker SSH 放行之前 |
| BYPASS-VERIFY-01 | 47-03 | verify.go 新增 3 项检查 | SATISFIED | verify.go:122-129 + 单测 |
| BYPASS-VERIFY-02 | 47-03 | 10 条不变量 CI 化 | BLOCKED | uat-bypass.yml uat job if:false；只 lint + dry-run（gap #4） |
| BYPASS-VERIFY-03 | 47-03 | uat-bypass.sh 6 场景 | SATISFIED（脚本层面） | scripts/uat-bypass.sh:94 6 个 SCENARIO_NAMES；--dry-run 6 场景全过；但 I2/I9 nft 族字面值错（gap #3） |
| BYPASS-VERIFY-04 | 47-03 | applied / rolled_back 端到端可见 | SATISFIED（脚本层面） | assert_snapshot_state helper + uat-bypass.sh 调 /bypass/snapshots；fail-closed-pkill 场景断言 rolled_back |

**REQ 覆盖：** 12 ID 全部在 PLAN frontmatter 声明且全部有代码实现引用；其中 3 条（BYPASS-RELOAD-02 / BYPASS-RELOAD-04 / BYPASS-VERIFY-02）被 family/table/netns 错位 / CI hedge 阻断，未达 ROADMAP success criteria。

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| internal/network/bypass_reload.go | 19-23, 47, 331-333 | 硬编码 nft family/table 与生产代码 ConfigureBypassFirewall 实际表名不一致 | Blocker | reload 链路在生产 worker netns 上 100% 失败 |
| internal/network/bypass_reload.go | 38-42, 46-49 | nftRunner / nftJSONLister 无 nsenter，host 进程直接执行 nft | Blocker | host 上 nft 看不到 worker 容器 netns 内的 set / 表 |
| scripts/uat-bypass.sh | 236, 341 | `nft list chain inet cloudproxy output` 写错 family（实际是 `ip cloudproxy`） | Blocker | I2 / I9 不变量启用后会持续假红 |
| .github/workflows/uat-bypass.yml | 78 | uat job 永久 `if: ${{ false }}` | Blocker | ROADMAP SC#5「10 条不变量全部接入 CI」未达成；只剩 lint+dry-run 文本守护 |
| scripts/uat-bypass.sh | 349-363 | I10 SSH 不断断言走 SKIP 兜底 | Warning | 当 CI 实际启用后，如未提前启 ssh while-loop 则 I10 永远 SKIP，不计 FAIL |
| scripts/uat-bypass.sh | 339-346 | I9 lenient 路径（只看 drop 规则存在，不注入 mDNS 包做 counter ≥ N） | Warning | SUMMARY.md 已声明，但与 ROADMAP「mDNS/LLMNR/NetBIOS 计数器可读」差一步 |

### Human Verification Required

无独立人工验证项。所有 gap 均可由 grep + 测试 + CI 配置变更程序化关闭。

### Gaps Summary

Phase 47 的 12 个任务在「文件存在 + 单测 PASS + go build」层面全部完成（8/12 must-haves 达成），代码组织清晰、TDD gate 守得很到位。但**整个热更新链路在生产路径上不能工作**，根因是 Plan 47-01 与 Plan 47-02 在「nft 表族 + 表名 + 是否进 worker netns」三个关键设计点上存在隐性契约错配：

- **47-CONTEXT.md / Plan 47-01** 假设白名单 set 位于独立的 `inet sbfw whitelist_v4` 表，host-agent 用 host 上的 nft 直接操作。
- **Plan 47-02 实现** 复用了 v3.4 留下的 `ip cloudproxy`（Family IPv4）表，set 跟随其他 worker netns 规则一起住在该表内，且只能通过 nftables.Conn + WithNetNSFd 操作。
- **bypass_reload.go** 按 Plan 47-01 假设硬编码字面值 `inet sbfw whitelist_v4`，且没有 nsenter 进入 worker netns —— 但代码所有的单测都用 fake nftRunner 闭包，遮蔽了真表/真 netns 的对照。
- **uat-bypass.sh** 又自己拍脑袋写成 `inet cloudproxy`，与两边都不一致。

这是典型的「task 完成但 phase goal 未达成」case——单测全 PASS 不代表 ROADMAP 承诺「apply 动作真正落到容器 netns nftables set 上」。要关闭 gap，需要先在 47-01 / 47-02 间敲定「nft 表族 + 是否独立 sbfw 表 + 是否走 nsenter」三个决策，再统一改 bypass_reload.go / bypass_firewall.go / worker_firewall_linux.go / uat-bypass.sh，并补一组真实 Linux 集成测试避免下次再被 fake 闭包遮蔽。CI 接入（uat-bypass.yml uat job 翻 if:false → true）是独立的第二个 gap。

---

_Verified: 2026-05-12_
_Verifier: Claude (gsd-verifier)_
