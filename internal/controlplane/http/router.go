package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	nethttp "net/http"
	"time"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/broadcast"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type EventRecorder interface {
	RecordEvent(context.Context, repository.RecordEventParams) (repository.Event, error)
}

type AdminEventStore interface {
	ListEvents(context.Context, repository.ListEventsParams) (repository.ListEventsResult, error)
}

type AgentHealthChecker interface {
	Ping(context.Context) error
}

type Dependencies struct {
	Logger          *slog.Logger
	Health          HealthChecker
	AgentHealth     AgentHealthChecker
	Users           UserLister
	Hosts           HostLister
	HostActions     HostActionQueuer
	Tasks           TaskLister
	TasksHandler    nethttp.Handler
	BootstrapUsers  BootstrapUserLookup
	BootstrapHosts  BootstrapHostLookup
	BootstrapTasks  TaskGetter
	BootstrapEvents EventLister
	ScriptPath      string
	Admin           *repository.AdminConfig
	AuthStore       AuthUserStore
	DashboardStats  DashboardStatsGetter
	AdminUsers      AdminUserStore
	AdminEgressIPs  AdminEgressIPStore
	AdminBindings   AdminBindingStore
	AdminBypassPresets  AdminBypassPresetStore
	AdminBypassRules    AdminBypassRuleStore
	AdminBypassBindings AdminBypassBindingStore
	AdminBypassProxy    AdminBypassProxyIPProvider // 通常复用 AdminEgressIPs
	// Phase 46 Plan 02：snapshot / audit-log 写读分离。
	AdminBypassSnapshots AdminBypassSnapshotStore
	AdminBypassAuditLog  AdminBypassAuditLogStore
	AdminHosts          AdminHostStore
	AdminClaudeAccounts AdminClaudeAccountStore // Phase 33 D-17
	AgentClient         HostActionRunner        // Phase 33 D-17 — interface 兼容 embedded + 远端两种模式
	AdminEvents         AdminEventStore
	EventRecorder   EventRecorder
	EntryStore      EntryStore
	EntryBaseURL    string
	ImageLockPath   string
	UserHosts       UserHostStore
	SSHKeys         SSHKeyStore
	ImageCache      ImageCacheManager
}

type HealthChecker interface {
	Health(context.Context) error
}

type UserLister interface {
	ListUsers(context.Context) ([]repository.User, error)
}

type HostLister interface {
	ListHosts(context.Context) ([]repository.Host, error)
}

type HostActionQueuer interface {
	// QueueHostAction 入队一个 host action。
	// 第 4 参 requestedBy 是触发者标识（admin UI / system / user-id），用于 audit。
	// 第 5 参 bypassSnapshotID 仅在 action=ActionReloadHostBypass 时承载 host_bypass_snapshots.id，
	// 其它 action 传 "" 即可（runtime_service 会按 action 类型 gating）。
	QueueHostAction(ctx context.Context, hostID string, action agentapi.HostAction, requestedBy string, bypassSnapshotID string) (repository.Task, error)
}

type TaskLister interface {
	ListTasksWithLastErrorSummary(context.Context) ([]repository.Task, error)
}

func NewRouter(deps Dependencies) nethttp.Handler {
	mux := nethttp.NewServeMux()
	tasksHandler := deps.TasksHandler
	if tasksHandler == nil {
		tasksHandler = NewTasksHandler(TasksHandlerDependencies{Logger: deps.Logger, Tasks: deps.Tasks})
	}
	hostActionsHandler := NewHostActionsHandler(HostActionHandlerDependencies{
		Logger: deps.Logger,
		Queue:  deps.HostActions,
	})

	mux.HandleFunc("GET /healthz", func(w nethttp.ResponseWriter, r *nethttp.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		checks := map[string]string{}
		status := nethttp.StatusOK

		if deps.Health != nil {
			if err := deps.Health.Health(ctx); err != nil {
				checks["database"] = err.Error()
				status = nethttp.StatusServiceUnavailable
			} else {
				checks["database"] = "ok"
			}
		}

		if deps.AgentHealth != nil {
			if err := deps.AgentHealth.Ping(ctx); err != nil {
				checks["agent"] = "unreachable"
			} else {
				checks["agent"] = "ok"
			}
		}

		overall := "ok"
		if status != nethttp.StatusOK {
			overall = "degraded"
		} else if checks["agent"] == "unreachable" {
			overall = "warning"
		}
		writeJSON(w, status, map[string]any{
			"status": overall,
			"checks": checks,
		})
	})

	mux.HandleFunc("GET /v1/users", func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if deps.Users == nil {
			writeJSON(w, nethttp.StatusServiceUnavailable, map[string]string{"error": "users repository unavailable"})
			return
		}

		users, err := deps.Users.ListUsers(r.Context())
		if err != nil {
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "list users failed"})
			return
		}

		writeJSON(w, nethttp.StatusOK, map[string]any{"users": users})
	})

	mux.HandleFunc("GET /v1/hosts", func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if deps.Hosts == nil {
			writeJSON(w, nethttp.StatusServiceUnavailable, map[string]string{"error": "hosts repository unavailable"})
			return
		}

		hosts, err := deps.Hosts.ListHosts(r.Context())
		if err != nil {
			writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "list hosts failed"})
			return
		}

		writeJSON(w, nethttp.StatusOK, map[string]any{"hosts": hosts})
	})

	bootstrapAuthHandler := NewBootstrapAuthHandler(BootstrapAuthDependencies{
		Logger: deps.Logger,
		Users:  deps.BootstrapUsers,
		Hosts:  deps.BootstrapHosts,
		Queue:  deps.HostActions,
		Events: deps.EventRecorder,
	})
	bootstrapScriptHandler := NewBootstrapScriptHandler(BootstrapScriptDependencies{
		Logger:     deps.Logger,
		ScriptPath: deps.ScriptPath,
	})

	mux.Handle("GET /v1/tasks", tasksHandler)
	mux.Handle("POST /v1/hosts/{hostID}/create", hostActionsHandler.Create())
	mux.Handle("POST /v1/hosts/{hostID}/start", hostActionsHandler.Start())
	mux.Handle("POST /v1/hosts/{hostID}/stop", hostActionsHandler.Stop())
	mux.Handle("POST /v1/hosts/{hostID}/rebuild", hostActionsHandler.Rebuild())
	bootstrapStatusHandler := NewBootstrapStatusHandler(BootstrapStatusDependencies{
		Logger: deps.Logger,
		Tasks:  deps.BootstrapTasks,
		Events: deps.BootstrapEvents,
	})

	bootstrapHandoffHandler := NewBootstrapHandoffHandler(BootstrapHandoffDependencies{
		Logger: deps.Logger,
		Tasks:  deps.BootstrapTasks,
		Events: deps.BootstrapEvents,
	})

	mux.Handle("POST /v1/bootstrap/sessions", bootstrapAuthHandler)
	mux.Handle("GET /v1/bootstrap/script", bootstrapScriptHandler)
	mux.Handle("GET /v1/bootstrap/tasks/{taskID}", bootstrapStatusHandler)
	mux.Handle("GET /v1/bootstrap/tasks/{taskID}/handoff", bootstrapHandoffHandler)

	if deps.EntryStore != nil {
		entryHandler := NewEntryHandler(deps.Logger, deps.EntryStore, deps.EntryBaseURL, deps.ImageLockPath)
		mux.Handle("GET /entry/{username}", entryHandler.Script())
		mux.Handle("POST /v1/entry/{username}/auth", entryHandler.Auth())
	}

	if deps.Admin != nil {
		loginHandler := NewUnifiedLoginHandler(deps.Logger, deps.AuthStore, deps.Admin.JWTSecret)
		mux.Handle("POST /v1/auth/login", loginHandler)
		mux.Handle("POST /v1/admin/login", loginHandler) // 兼容旧端点

		authMw := AuthMiddleware(deps.Admin.JWTSecret)
		adminGuard := func(h nethttp.Handler) nethttp.Handler {
			return authMw(RequireRole("admin")(h))
		}
		dashboardHandler := NewDashboardHandler(deps.Logger, deps.DashboardStats)
		mux.Handle("GET /v1/admin/dashboard/stats", adminGuard(dashboardHandler))

		if deps.AdminUsers != nil {
			usersHandler := NewAdminUsersHandler(deps.Logger, deps.AdminUsers, deps.EventRecorder)
			mux.Handle("GET /v1/admin/users", adminGuard(usersHandler.List()))
			mux.Handle("POST /v1/admin/users", adminGuard(usersHandler.Create()))
			mux.Handle("GET /v1/admin/users/{userID}", adminGuard(usersHandler.Get()))
			mux.Handle("PATCH /v1/admin/users/{userID}", adminGuard(usersHandler.UpdateStatus()))
			mux.Handle("DELETE /v1/admin/users/{userID}", adminGuard(usersHandler.Delete()))
			mux.Handle("POST /v1/admin/users/{userID}/rotate-password", adminGuard(usersHandler.RotatePassword()))
			mux.Handle("POST /v1/admin/users/{userID}/credentials/regenerate", adminGuard(usersHandler.RegenerateCredentials()))
			mux.Handle("PUT /v1/admin/users/{userID}/expiry", adminGuard(usersHandler.UpdateExpiry()))
		}

		if deps.AdminEgressIPs != nil {
			egressHandler := NewAdminEgressIPsHandler(deps.Logger, deps.AdminEgressIPs, deps.EventRecorder)
			mux.Handle("GET /v1/admin/egress-ips", adminGuard(egressHandler.List()))
			mux.Handle("POST /v1/admin/egress-ips", adminGuard(egressHandler.Create()))
			mux.Handle("GET /v1/admin/egress-ips/{ipID}", adminGuard(egressHandler.Get()))
			mux.Handle("PUT /v1/admin/egress-ips/{ipID}", adminGuard(egressHandler.Update()))
			mux.Handle("DELETE /v1/admin/egress-ips/{ipID}", adminGuard(egressHandler.Delete()))
			mux.Handle("POST /v1/admin/egress-ips/{ipID}/test", adminGuard(egressHandler.TestProxy()))
			mux.Handle("GET /v1/admin/egress-ips/{ipID}/test/stream", adminGuard(egressHandler.TestProxyStream()))
		}

		if deps.AdminBindings != nil {
			bindingsHandler := NewAdminBindingsHandler(deps.Logger, deps.AdminBindings, deps.EventRecorder)
			mux.Handle("POST /v1/admin/bindings", adminGuard(bindingsHandler.Bind()))
			mux.Handle("DELETE /v1/admin/bindings/{bindingID}", adminGuard(bindingsHandler.Unbind()))
		}

		// Phase 46 Plan 01：Bypass Preset / Rule / Binding 后台 API
		if deps.AdminBypassPresets != nil {
			presetHandler := NewAdminBypassPresetsHandler(deps.Logger, deps.AdminBypassPresets, deps.EventRecorder)
			mux.Handle("GET /v1/admin/bypass/presets", adminGuard(presetHandler.List()))
			mux.Handle("POST /v1/admin/bypass/presets", adminGuard(presetHandler.Create()))
			mux.Handle("GET /v1/admin/bypass/presets/{presetID}", adminGuard(presetHandler.Get()))
			mux.Handle("PATCH /v1/admin/bypass/presets/{presetID}", adminGuard(presetHandler.Update()))
			mux.Handle("DELETE /v1/admin/bypass/presets/{presetID}", adminGuard(presetHandler.Delete()))
		}

		if deps.AdminBypassRules != nil {
			// 若未显式注入 ProxyIPProvider，退化到 AdminEgressIPs（接口子集兼容）。
			var proxy AdminBypassProxyIPProvider = deps.AdminBypassProxy
			if proxy == nil && deps.AdminEgressIPs != nil {
				proxy = deps.AdminEgressIPs
			}
			ruleHandler := NewAdminBypassRulesHandler(deps.Logger, deps.AdminBypassRules, proxy, deps.EventRecorder)
			mux.Handle("GET /v1/admin/bypass/rules", adminGuard(ruleHandler.List()))
			mux.Handle("POST /v1/admin/bypass/rules", adminGuard(ruleHandler.Create()))
			mux.Handle("POST /v1/admin/bypass/rules/validate", adminGuard(ruleHandler.Validate()))
			mux.Handle("PATCH /v1/admin/bypass/rules/{ruleID}", adminGuard(ruleHandler.Update()))
			mux.Handle("DELETE /v1/admin/bypass/rules/{ruleID}", adminGuard(ruleHandler.Delete()))
		}

		if deps.AdminBypassBindings != nil {
			bypassBindingHandler := NewAdminBypassBindingsHandler(deps.Logger, deps.AdminBypassBindings, deps.EventRecorder)
			mux.Handle("GET /v1/admin/hosts/{hostID}/bypass", adminGuard(bypassBindingHandler.ListByHost()))
			mux.Handle("POST /v1/admin/hosts/{hostID}/bypass", adminGuard(bypassBindingHandler.Bind()))
			mux.Handle("DELETE /v1/admin/bypass/bindings/{bindingID}", adminGuard(bypassBindingHandler.Unbind()))
		}

		// Phase 46 Plan 02：snapshot 写动作 + audit log 读端点。
		if deps.AdminBypassSnapshots != nil {
			sh := NewAdminBypassSnapshotsHandler(deps.Logger, deps.AdminBypassSnapshots, deps.HostActions, deps.EventRecorder)
			mux.Handle("POST /v1/admin/hosts/{hostID}/bypass/preview", adminGuard(sh.Preview()))
			mux.Handle("POST /v1/admin/hosts/{hostID}/bypass/apply", adminGuard(sh.Apply()))
			mux.Handle("POST /v1/admin/hosts/{hostID}/bypass/rollback", adminGuard(sh.Rollback()))
			mux.Handle("GET /v1/admin/hosts/{hostID}/bypass/effective", adminGuard(sh.Effective()))
			mux.Handle("GET /v1/admin/hosts/{hostID}/bypass/consistency", adminGuard(sh.Consistency()))
		}
		if deps.AdminBypassAuditLog != nil {
			ah := NewAdminBypassAuditLogHandler(deps.Logger, deps.AdminBypassAuditLog)
			mux.Handle("GET /v1/admin/hosts/{hostID}/bypass/audit-log", adminGuard(ah.ListByHost()))
		}

		if deps.AdminHosts != nil {
			hostsHandler := NewAdminHostsHandler(deps.Logger, deps.AdminHosts, deps.HostActions, deps.EventRecorder, deps.ImageLockPath)
			mux.Handle("GET /v1/admin/hosts", adminGuard(hostsHandler.List()))
			mux.Handle("POST /v1/admin/hosts", adminGuard(hostsHandler.Create()))
			mux.Handle("GET /v1/admin/hosts/{hostID}", adminGuard(hostsHandler.Get()))
			mux.Handle("POST /v1/admin/hosts/{hostID}/start", adminGuard(hostsHandler.Start()))
			mux.Handle("POST /v1/admin/hosts/{hostID}/stop", adminGuard(hostsHandler.Stop()))
			mux.Handle("POST /v1/admin/hosts/{hostID}/rebuild", adminGuard(hostsHandler.Rebuild()))
			mux.Handle("POST /v1/admin/hosts/{hostID}/vnc/restart", adminGuard(hostsHandler.RestartVNC()))
			mux.Handle("POST /v1/admin/hosts/{hostID}/change-root-password", adminGuard(hostsHandler.ChangeRootPassword()))
			mux.Handle("GET /v1/admin/hosts/{hostID}/claude/settings", adminGuard(hostsHandler.GetClaudeSettings()))
			mux.Handle("PUT /v1/admin/hosts/{hostID}/claude/settings", adminGuard(hostsHandler.UpdateClaudeSettings()))
			mux.Handle("GET /v1/admin/hosts/{hostID}/claude/status", adminGuard(hostsHandler.GetClaudeStatus()))
			mux.Handle("GET /v1/admin/hosts/{hostID}/claude/info", adminGuard(hostsHandler.GetClaudeInfo()))
			mux.Handle("POST /v1/admin/hosts/{hostID}/claude/update", adminGuard(hostsHandler.UpdateClaude()))
			mux.Handle("GET /v1/admin/hosts/{hostID}/config/export", adminGuard(hostsHandler.ExportConfig()))
			mux.Handle("POST /v1/admin/hosts/{hostID}/config/import", adminGuard(hostsHandler.ImportConfig()))
			mux.Handle("GET /v1/admin/hosts/{hostID}/image-info", adminGuard(hostsHandler.GetImageInfo()))
			mux.Handle("GET /v1/admin/hosts/{hostID}/logs", adminGuard(hostsHandler.GetLogs()))
			mux.Handle("DELETE /v1/admin/hosts/{hostID}", adminGuard(hostsHandler.Delete()))
			mux.Handle("PUT /v1/admin/hosts/{hostID}/mounts", adminGuard(hostsHandler.UpdateMounts()))

			vncProxy := NewAdminVNCProxyHandler(deps.Logger, deps.AdminHosts)
			// VNC 入口页 (vnc.html) 带 ?token= 认证；子资源（CSS/JS/图片）
			// 只是 KasmVNC 通用 UI，安全边界在 WebSocket 连接，无需逐个认证。
			mux.Handle("/v1/admin/hosts/{hostID}/vnc/{path...}", vncProxy)
		}

		if deps.AdminClaudeAccounts != nil {
			claudeHandler := NewAdminClaudeAccountsHandler(deps.Logger, deps.AdminClaudeAccounts, deps.AgentClient, deps.EventRecorder)
			mux.Handle("DELETE /v1/admin/claude-accounts/{accountID}", adminGuard(claudeHandler.Delete()))
		}

			if deps.ImageCache != nil {
				imageHandler := NewAdminImageHandler(deps.Logger, deps.ImageCache)
				mux.Handle("GET /v1/admin/image/status", adminGuard(imageHandler.Status()))
				mux.Handle("POST /v1/admin/image/refresh", adminGuard(imageHandler.Refresh()))
			}

		if deps.AdminEvents != nil {
			eventsHandler := NewAdminEventsHandler(deps.Logger, deps.AdminEvents)
			mux.Handle("GET /v1/admin/events", adminGuard(eventsHandler.List()))
		}

		// SSE 实时推送端点 —— 必须套 adminGuard（CR-07）。
		// EventSource 不能附 Authorization header，前端用 ?token=... 方式认证；
		// AuthMiddleware.extractToken 已支持从 query param 提取，无需额外改造。
		mux.Handle("GET /v1/admin/sse", adminGuard(nethttp.HandlerFunc(broadcast.Subscribe)))

		if deps.SSHKeys != nil {
			sshKeyHandler := NewSSHKeyHandler(deps.Logger, deps.SSHKeys)
			mux.Handle("GET /v1/admin/users/{userID}/ssh-keys", adminGuard(sshKeyHandler.List()))
			mux.Handle("POST /v1/admin/users/{userID}/ssh-keys", adminGuard(sshKeyHandler.Create()))
			mux.Handle("DELETE /v1/admin/users/{userID}/ssh-keys/{keyID}", adminGuard(sshKeyHandler.Delete()))
		}

		mux.Handle("GET /v1/admin/host-files", adminGuard(NewAdminHostFilesHandler(deps.Logger).List()))
		mux.Handle("GET /v1/admin/tasks", adminGuard(tasksHandler))

		// User self-service endpoints (D-01: /v1/user/ prefix, D-02: user+admin roles)
		userGuard := func(h nethttp.Handler) nethttp.Handler {
			return authMw(RequireRole("user", "admin")(h))
		}

		if deps.AdminUsers != nil {
			userPasswordHandler := NewUserPasswordHandler(deps.Logger, deps.AdminUsers)
			mux.Handle("POST /v1/user/change-password", userGuard(userPasswordHandler.ChangePassword()))
		}

		if deps.UserHosts != nil {
			userHostsHandler := NewUserHostsHandler(deps.Logger, deps.UserHosts, deps.HostActions, deps.EventRecorder)
			mux.Handle("GET /v1/user/hosts", userGuard(userHostsHandler.List()))
			mux.Handle("GET /v1/user/hosts/{hostID}", userGuard(userHostsHandler.Get()))
			mux.Handle("POST /v1/user/hosts/{hostID}/rebuild", userGuard(userHostsHandler.Rebuild()))
			mux.Handle("POST /v1/user/hosts/{hostID}/vnc/restart", userGuard(userHostsHandler.RestartVNC()))

			userVNCProxy := NewUserVNCProxyHandler(deps.Logger, deps.UserHosts)
			mux.Handle("/v1/user/hosts/{hostID}/vnc/{path...}", userGuard(userVNCProxy))
		}

		// 用户门户 SSE 实时推送端点 —— 同样要求 userGuard 鉴权（CR-07）。
		mux.Handle("GET /v1/user/sse", userGuard(nethttp.HandlerFunc(broadcast.Subscribe)))

		if deps.SSHKeys != nil {
			userSSHKeyHandler := NewSSHKeyHandler(deps.Logger, deps.SSHKeys)
			mux.Handle("GET /v1/user/ssh-keys", userGuard(userSSHKeyHandler.List()))
			mux.Handle("POST /v1/user/ssh-keys", userGuard(userSSHKeyHandler.Create()))
			mux.Handle("DELETE /v1/user/ssh-keys/{keyID}", userGuard(userSSHKeyHandler.Delete()))
		}
	}

	return mux
}

func writeJSON(w nethttp.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil && !errors.Is(err, nethttp.ErrHandlerTimeout) {
		nethttp.Error(w, `{"error":"encode response failed"}`, nethttp.StatusInternalServerError)
	}
}
