# Requirements: Cloud CLI Proxy

**Defined:** 2026-04-03
**Core Value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP，同时保持"一条命令启动"的体验足够顺滑。

## v1.3 Requirements

Requirements for v1.3 管理后台凭据与 SSH 密钥增强。Each maps to roadmap phases.

### 时区与交互

- [ ] **TZ-01**: 管理员在所有时区选择器中可以选择完整时区列表，列表覆盖系统支持的全部 IANA 时区
- [ ] **TZ-02**: 所有时区选项都显示 UTC 偏移标识（例如 UTC-08:00、UTC+08:00），并保持与时区值一致
- [ ] **UI-01**: 管理员创建出口 IP 时使用 Dialog 弹窗交互，不再使用右侧抽屉
- [ ] **UI-02**: 管理员编辑出口 IP 时使用 Dialog 弹窗交互，原有字段校验与提交能力保持可用

### 用户密码管理

- [ ] **CRED-01**: 管理员创建用户时可以手动填写登录密码
- [ ] **CRED-02**: 管理员创建用户时可以手动填写 SSH 密码
- [ ] **CRED-03**: 创建用户时若登录密码或 SSH 密码留空，系统会为留空项自动随机生成密码
- [ ] **CRED-04**: 管理员在用户详情页可以修改登录密码与 SSH 密码，每项都支持手填或一键随机生成
- [ ] **CRED-05**: 用户详情页提交密码修改时，目标密码项必须有有效值（手填或系统生成）才允许提交

### SSH 密钥模型与同步

- [ ] **KEYA-01**: 系统区分并管理登录授权公钥（用于用户进入该容器）
- [ ] **KEYA-02**: 每个用户可维护多条登录授权公钥，并支持新增、更新、删除
- [ ] **KEYI-01**: 系统区分并管理外联身份密钥对（用于容器连接外部主机、提交代码等）
- [ ] **KEYI-02**: 每个用户可维护多组外联身份密钥对，并支持新增、更新、删除
- [ ] **KEYS-01**: 运行中容器在 SSH 密钥变更后可实时同步（新增、更新、删除）
- [ ] **KEYS-02**: 非运行中容器在下次启动时会补齐最新 SSH 密钥状态，确保容器内外配置一致

## Future Requirements

### 用户接入体验（延期）

- **BOOT-01**: 用户通过 `curl domain/{short_id}` 进入引导流程
- **BOOT-02**: 引导脚本展示产品名称 ASCII 艺术字欢迎界面
- **BOOT-03**: 容器启动过程通过 SSE 实时推送状态到终端
- **BOOT-04**: 启动完成后自动建立 SSH 连接进入容器

### 用户自助面板（延期）

- **PANEL-04**: 用户可在自助面板通过浏览器直接访问 KasmVNC 远程桌面
- **CLAUDE-01**: 系统支持一个用户拥有多个 Claude 账号，每个账号对应一台主机
- **CLAUDE-02**: 管理员可创建、编辑、删除 Claude 账号并绑定到用户和主机
- **CLAUDE-03**: 用户可在自助面板查看自己的 Claude 账号信息

## Out of Scope

| Feature | Reason |
|---------|--------|
| 计费、套餐、余额和自助支付 | 在核心主机生命周期和网络强约束能力验证前不纳入 |
| 多宿主机编排 | v1 限制为单宿主机 |
| 用户自定义任意镜像 | 削弱就绪性、安全性和可支持性 |
| 用户自选代理节点 | 由管理员统一配置 |
| 代理链/多跳 | 增加延迟和排障复杂度，非当前核心目标 |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| TZ-01 | Phase 17 | Pending |
| TZ-02 | Phase 17 | Pending |
| UI-01 | Phase 17 | Pending |
| UI-02 | Phase 17 | Pending |
| CRED-01 | Phase 18 | Pending |
| CRED-02 | Phase 18 | Pending |
| CRED-03 | Phase 18 | Pending |
| CRED-04 | Phase 18 | Pending |
| CRED-05 | Phase 18 | Pending |
| KEYA-01 | Phase 19 | Pending |
| KEYA-02 | Phase 19 | Pending |
| KEYI-01 | Phase 19 | Pending |
| KEYI-02 | Phase 19 | Pending |
| KEYS-01 | Phase 20 | Pending |
| KEYS-02 | Phase 20 | Pending |

**Coverage:**
- v1.3 requirements: 15 total
- Mapped to phases: 15
- Unmapped: 0

---
*Requirements defined: 2026-04-03*
*Last updated: 2026-04-03 after milestone scoping*
