package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/broadcast"
	cphttp "github.com/zanel1u/cloud-cli-proxy/internal/controlplane/http"
	"github.com/zanel1u/cloud-cli-proxy/internal/controlplane/scheduler"
	"github.com/zanel1u/cloud-cli-proxy/internal/network"
	"github.com/zanel1u/cloud-cli-proxy/internal/runtime"
	runtimetasks "github.com/zanel1u/cloud-cli-proxy/internal/runtime/tasks"
	"github.com/zanel1u/cloud-cli-proxy/internal/sshproxy"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/migrator"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type Config struct {
	Addr               string
	DatabaseURL        string
	MigrationDir       string
	AdminUsername       string
	AdminPassword      string
	AdminJWTSecret     string
	ExpiryScanInterval time.Duration
	ReconcileInterval  time.Duration

	SSHProxyAddr              string
	SSHProxyContainerUser     string
	SSHProxyContainerPassword string
	SSHProxyHostKeyPath       string
}

type App struct {
	cfg           Config
	logger        *slog.Logger
	db            *pgxpool.Pool
	repo          *repository.Repository
	migrator      func(context.Context, *pgxpool.Pool, string) error
	handler       http.Handler
	expiryScanner *scheduler.ExpiryScanner
	reconciler    *scheduler.Reconciler
	imageCache    *runtime.ImageCache
	sshProxy      *sshproxy.Server
}

func newLogger() *slog.Logger {
	level := slog.LevelInfo
	if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
		var l slog.Level
		if err := l.UnmarshalText([]byte(lvl)); err == nil {
			level = l
		}
	}
	opts := &slog.HandlerOptions{Level: level}
	if os.Getenv("LOG_FORMAT") == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

func New(ctx context.Context, cfg Config) (*App, error) {
	logger := newLogger()
	broadcast.SetLogger(logger.With("component", "sse"))

	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	poolCfg.MaxConns = 10
	poolCfg.MinConns = 2
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute
	db, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}

	repo := repository.New(db)

	type dispatcherIface interface {
		Dispatch(context.Context, agentapi.HostActionRequest) (agentapi.HostActionResponse, error)
	}

	var dispatcher dispatcherIface
	var agentHealth cphttp.AgentHealthChecker
	var agentRunner cphttp.HostActionRunner // Phase 33: admin DELETE claude-accounts handler 用

	embeddedMode := os.Getenv("HOST_AGENT_MODE") == "embedded"
	if embeddedMode {
		logger.Info("host-agent mode: embedded (in-process worker)")
		worker := runtimetasks.NewWorker(repo, network.NewProvider(logger))
		embeddedDisp := runtimetasks.NewEmbeddedDispatcher(worker)
		dispatcher = embeddedDisp
		agentRunner = embeddedDisp // EmbeddedDispatcher 实现 RunHostAction 适配器
	} else {
		socketPath := agentapi.DefaultSocketPath
		if p := os.Getenv("HOST_AGENT_SOCKET"); p != "" {
			socketPath = p
		}
		agentClient := agentapi.NewClient(socketPath)
		dispatcher = runtimetasks.NewDispatcher(agentClient)
		agentHealth = agentClient
		agentRunner = agentClient
	}

	runtimeService := runtime.NewService(repo, dispatcher, runtime.DefaultImageLockPath)
	imageCache := runtime.NewImageCache(logger, runtime.DefaultImageLockPath)

	var adminCfg *repository.AdminConfig
	if cfg.AdminJWTSecret != "" {
		adminCfg = &repository.AdminConfig{
			Username:  cfg.AdminUsername,
			Password:  cfg.AdminPassword,
			JWTSecret: []byte(cfg.AdminJWTSecret),
		}
	}

	expiryScanner := scheduler.NewExpiryScanner(logger, repo, runtimeService)

	var reconciler *scheduler.Reconciler
	if !embeddedMode {
		socketPath := agentapi.DefaultSocketPath
		if p := os.Getenv("HOST_AGENT_SOCKET"); p != "" {
			socketPath = p
		}
		reconciler = scheduler.NewReconciler(logger, repo, agentapi.NewClient(socketPath), runtimeService, 0)
	} else {
		// embedded 模式下没有 host-agent socket，inspector 直接调用 docker
		reconciler = scheduler.NewReconciler(logger, repo, &dockerInspector{}, runtimeService, 0)
	}

	router := cphttp.NewRouter(cphttp.Dependencies{
		Logger:         logger,
		Health:         repo,
		AgentHealth:    agentHealth,
		Users:          repo,
		Hosts:          repo,
		HostActions:    runtimeService,
		Tasks:          repo,
		TasksHandler:   cphttp.NewTasksHandler(cphttp.TasksHandlerDependencies{Logger: logger, Tasks: repo}),
		Admin:          adminCfg,
		AuthStore:      repo,
		DashboardStats: repo,
		AdminUsers:     repo,
		AdminEgressIPs: repo,
		AdminBindings:  repo,
		AdminHosts:          repo,
		AdminClaudeAccounts: repo, // Phase 33: 复用 Repository.BeginTx 满足 AdminClaudeAccountStore
		AgentClient:         agentRunner,
		AdminEvents:         repo,
		EventRecorder:       repo,
		EntryStore:     repo,
		EntryBaseURL:   "",
		ImageLockPath:  runtime.DefaultImageLockPath,
		UserHosts:      repo,
		SSHKeys:        repo,
		ImageCache:     imageCache,
	})

	var sshProxySrv *sshproxy.Server
	if cfg.SSHProxyAddr != "" {
		resolver := sshproxy.NewRepoResolver(repo)
		srv, err := sshproxy.NewServer(
			cfg.SSHProxyAddr,
			cfg.SSHProxyContainerUser,
			cfg.SSHProxyContainerPassword,
			cfg.SSHProxyHostKeyPath,
			resolver,
			logger.With("component", "ssh-proxy"),
		)
		if err != nil {
			return nil, fmt.Errorf("create ssh proxy: %w", err)
		}
		sshProxySrv = srv
	}

	return &App{
		cfg:           cfg,
		logger:        logger,
		db:            db,
		repo:          repo,
		migrator:      migrator.RunMigrations,
		handler:       router,
		expiryScanner: expiryScanner,
		reconciler:    reconciler,
		imageCache:    imageCache,
		sshProxy:      sshProxySrv,
	}, nil
}

func (a *App) ensureSeedAdmin(ctx context.Context) error {
	return ensureSeedAdminWithRepo(ctx, a.logger, a.repo, a.cfg.AdminUsername, a.cfg.AdminPassword)
}

func (a *App) Run(ctx context.Context) error {
	defer a.db.Close()

	if err := a.migrator(ctx, a.db, a.cfg.MigrationDir); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	if err := a.ensureSeedAdmin(ctx); err != nil {
		return fmt.Errorf("ensure seed admin: %w", err)
	}

	expiryInterval := a.cfg.ExpiryScanInterval
	if expiryInterval == 0 {
		expiryInterval = 60 * time.Second
	}

	reconcileInterval := a.cfg.ReconcileInterval
	if reconcileInterval == 0 {
		reconcileInterval = 60 * time.Second
	}

	jobs := []scheduler.Job{
		{Name: "expiry-scan", Interval: expiryInterval, Fn: a.expiryScanner.Scan},
	}
	if a.reconciler != nil {
		jobs = append(jobs, scheduler.Job{Name: "reconcile", Interval: reconcileInterval, Fn: a.reconciler.Run})
	}
	if a.imageCache != nil {
		jobs = append(jobs, scheduler.Job{Name: "image-cache-refresh", Interval: 30 * time.Minute, Fn: a.imageCache.Refresh})
		// 启动时立即执行一次刷新，使前端立即可获取镜像状态
		go func() {
			if err := a.imageCache.Refresh(ctx); err != nil {
				a.logger.Warn("initial image cache refresh failed", "error", err)
			}
		}()
	}
	sched := scheduler.New(a.logger, jobs)

	schedCtx, schedCancel := context.WithCancel(ctx)
	schedDone := make(chan struct{})
	go func() {
		sched.Run(schedCtx)
		close(schedDone)
	}()

	cphttp.CleanupOrphanProbes(a.logger)

	a.rejoinHostNetworks()

	if a.sshProxy != nil {
		go func() {
			if err := a.sshProxy.ListenAndServe(ctx); err != nil {
				a.logger.Error("SSH proxy stopped with error", "error", err)
			}
		}()
	}

	server := &http.Server{
		Addr:              a.cfg.Addr,
		Handler:           a.handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("control-plane listening", "addr", a.cfg.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}

		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			a.logger.Error("shutdown server failed", "error", err)
		}

		schedCancel()
		<-schedDone

		return ctx.Err()
	case err := <-errCh:
		schedCancel()
		<-schedDone
		return err
	}
}

func (a *App) rejoinHostNetworks() {
	cpID, _ := os.Hostname()
	if cpID == "" {
		a.logger.Warn("rejoin-networks: cannot determine hostname, skipping")
		return
	}

	// 探测控制面是否跑在 docker 容器内（hostname = 容器名）。
	// 非容器环境（如 macOS 宿主机直跑）直接跳过，避免 "No such container" 误报。
	inspectCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	if out, err := exec.CommandContext(inspectCtx, "docker", "inspect", "--format", "{{.Id}}", cpID).Output(); err != nil || len(strings.TrimSpace(string(out))) == 0 {
		cancel()
		return
	}
	cancel()

	cmd := exec.CommandContext(context.Background(), "docker", "network", "ls",
		"--filter", "name=cloudproxy-net-", "--format", "{{.Name}}")
	out, err := cmd.Output()
	if err != nil {
		a.logger.Warn("rejoin-networks: list networks failed", "error", err)
		return
	}

	networks := strings.Fields(strings.TrimSpace(string(out)))
	if len(networks) == 0 {
		return
	}

	joined := 0
	for _, net := range networks {
		connectCmd := exec.CommandContext(context.Background(), "docker", "network", "connect", net, cpID)
		if connectOut, err := connectCmd.CombinedOutput(); err != nil {
			msg := strings.TrimSpace(string(connectOut))
			if !strings.Contains(msg, "already exists") {
				a.logger.Warn("rejoin-networks: connect failed", "network", net, "error", msg)
			}
		} else {
			joined++
		}
	}

	if joined > 0 {
		a.logger.Info("rejoin-networks: connected to host networks", "count", joined, "total", len(networks))
	}
}

// dockerInspector 在 embedded 模式下直接调用 docker container inspect，
// 替代通过 host-agent socket 通信的 agentapi.Client。
type dockerInspector struct{}

func (d *dockerInspector) InspectContainer(ctx context.Context, containerName string) (agentapi.ContainerStatusResponse, error) {
	cmd := exec.CommandContext(ctx, "docker", "container", "inspect",
		"-f", "{{.State.Running}}", containerName)
	out, err := cmd.Output()
	if err != nil {
		return agentapi.ContainerStatusResponse{
			Name:    containerName,
			Exists:  false,
			Running: false,
		}, nil
	}

	running := strings.TrimSpace(string(out)) == "true"
	return agentapi.ContainerStatusResponse{
		Name:    containerName,
		Exists:  true,
		Running: running,
	}, nil
}
