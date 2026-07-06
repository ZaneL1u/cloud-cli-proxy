import { useRef, useState } from "react";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  Check,
  Copy,
  KeyRound,
  Monitor,
  Pause,
  Play,
  RefreshCw,
  Settings,
  Square,
  Terminal,
  Trash2,
  Clock,
  Globe,
  HardDrive,
  ExternalLink,
  ChevronDown,
  ArrowUpCircle,
  Loader2,
  CheckCircle2,
  XCircle,
  ChevronUp,
  Download,
  Upload,
  FolderOpen,
} from "lucide-react";
import { toast } from "sonner";
import { getToken } from "@/lib/auth";
import {
  useHostDetail,
  useHostImageInfo,
  useImportHostConfig,
  useExportHostConfig,
  useHostLogs,
  useHostAction,
  useDeleteHost,
  usePatchHostResources,
} from "@/hooks/use-hosts";
import { useTaskPolling } from "@/hooks/use-tasks";
import { useSSE } from "@/hooks/use-sse";
import { buildSSEUrl } from "@/lib/sse-manager";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { BindingManager } from "@/components/hosts/binding-manager";
import { MountManager } from "@/components/hosts/mount-manager";
import { ResourceLimitsSelector, type ResourceLimitsValue } from "@/components/hosts/resource-limits-selector";
import { RotatePasswordDialog } from "@/components/users/rotate-password-dialog";
import { ChangeRootPasswordDialog } from "@/components/hosts/change-root-password-dialog";
import { ClaudeSettingsDialog } from "@/components/hosts/claude-settings-dialog";
import { ClaudeStatusCard } from "@/components/hosts/claude-status-card";
import { RebuildDialog } from "@/components/hosts/rebuild-dialog";
import { BypassTab } from "@/components/bypass/bypass-tab";
import { HostFilesBrowser } from "@/components/hosts/host-files-browser";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";

export const Route = createFileRoute("/_dashboard/hosts/$hostId")({
  component: HostDetailPage,
});

function timeSince(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const minutes = Math.floor(diff / 60000);
  if (minutes < 60) return `${minutes} 分钟`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} 小时`;
  return `${Math.floor(hours / 24)} 天`;
}

const statusConfig: Record<string, { label: string; color: string; dot: string; bg: string; border: string }> = {
  running: { label: "运行中", color: "text-emerald-700", dot: "bg-emerald-500", bg: "bg-emerald-50", border: "border-emerald-200" },
  stopped: { label: "已停止", color: "text-slate-600", dot: "bg-slate-400", bg: "bg-slate-50", border: "border-slate-200" },
  pending: { label: "等待中", color: "text-amber-700", dot: "bg-amber-500", bg: "bg-amber-50", border: "border-amber-200" },
  failed: { label: "失败", color: "text-red-700", dot: "bg-red-500", bg: "bg-red-50", border: "border-red-200" },
};

type ConnTab = "ssh" | "curl" | "vnc";

function HostDetailPage() {
  const { hostId } = Route.useParams();
  const { data, isLoading } = useHostDetail(hostId);
  const { data: imageInfo } = useHostImageInfo(hostId, !!data?.host);
  const importConfigMutation = useImportHostConfig();
  const exportConfigMutation = useExportHostConfig();
  const actionMutation = useHostAction();
  const deleteMutation = useDeleteHost();
  const patchResourcesMutation = usePatchHostResources(hostId);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [activeTab, setActiveTab] = useState<ConnTab>("ssh");
  const [configOpen, setConfigOpen] = useState(true);
  const [rotateLoginOpen, setRotateLoginOpen] = useState(false);
  const [changeRootPwOpen, setChangeRootPwOpen] = useState(false);
  const [claudeSettingsOpen, setClaudeSettingsOpen] = useState(false);
  const [rebuildOpen, setRebuildOpen] = useState(false);
  const [filesOpen, setFilesOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [forceDeleteOpen, setForceDeleteOpen] = useState(false);
  const [upgradeOpen, setUpgradeOpen] = useState(false);
  const [upgradeTaskId, setUpgradeTaskId] = useState<string | null>(null);
  const [progressLayers, setProgressLayers] = useState<Record<string, string>>({});
  const [showLayers, setShowLayers] = useState(false);
  const [editingResources, setEditingResources] = useState(false);
  const [editResourcesValue, setEditResourcesValue] = useState<ResourceLimitsValue>({
    pids_limit: null,
    memory_limit_mb: null,
    cpu_limit: null,
  });

  const { data: task } = useTaskPolling(upgradeTaskId);

  useSSE(buildSSEUrl("/v1/admin/sse", "tasks", getToken()), (msg) => {
    if (msg.topic === "tasks" && msg.action === "progress" && msg.id === upgradeTaskId) {
      const payload = msg.payload as {
        percent?: number;
        message?: string;
        layers?: Record<string, string>;
      } | undefined;
      if (payload?.layers) {
        setProgressLayers(payload.layers);
      }
    }
  });

  const taskStatus = task?.status;
  const isUpgradeRunning = upgradeTaskId !== null && taskStatus !== "succeeded" && taskStatus !== "failed" && taskStatus !== "canceled";
  const isUpgradeDone = taskStatus === "succeeded" || taskStatus === "failed" || taskStatus === "canceled";

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="h-10 w-56 animate-pulse rounded-lg bg-muted" />
        <div className="h-52 animate-pulse rounded-xl bg-muted" />
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          {[1, 2, 3, 4].map((i) => (
            <div key={i} className="h-28 animate-pulse rounded-xl bg-muted" />
          ))}
        </div>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="py-20 text-center text-muted-foreground">
        主机不存在
      </div>
    );
  }

  const { host, user, bindings } = data;
  const sc = statusConfig[host.status] || { label: host.status, color: "text-slate-600", dot: "bg-slate-400", bg: "bg-slate-50", border: "border-slate-200" };
  const isRunning = host.status === "running";
  const displayName = host.hostname || user.username || host.id.slice(0, 8);

  const sshPort = data.connection_info?.ssh_port;
  const egressIP = bindings?.[0]?.egress_ip?.detected_ip_address
  || (bindings?.[0]?.egress_ip?.ip_address !== "0.0.0.0" ? bindings?.[0]?.egress_ip?.ip_address : undefined);

  function openVNC() {
    const token = getToken();
    const wsPath = encodeURIComponent(`v1/admin/hosts/${host.id}/vnc/`);
    window.open(
      `/v1/admin/hosts/${host.id}/vnc/vnc.html?autoconnect=true&resize=remote&path=${wsPath}&token=${token}`,
      "_blank",
    );
  }

  function handleAction(action: "start" | "stop") {
    actionMutation.mutate(
      { hostId, action },
      {
        onSuccess: () => toast.success(action === "start" ? "启动指令已发送" : "停止指令已发送"),
        onError: () => toast.error("操作失败"),
      },
    );
  }

  function handleDelete(force: boolean) {
    deleteMutation.mutate(
      { hostId, force },
      {
        onSuccess: () => {
          toast.success("主机已删除");
          window.location.href = "/hosts";
        },
        onError: (err: any) => toast.error(err?.message || "删除失败"),
      },
    );
  }

  const connTabs: { key: ConnTab; label: string; value?: string }[] = [
    { key: "ssh", label: "SSH", value: data.connection_info?.ssh_command },
    { key: "curl", label: "一键连接", value: data.connection_info?.curl_command },
    ...(data.connection_info?.vnc_url ? [{ key: "vnc" as ConnTab, label: "VNC", value: data.connection_info.vnc_url }] : []),
  ];

  const activeConn = connTabs.find((t) => t.key === activeTab) ?? connTabs[0];

  return (
    <div className="space-y-6">
      {/* ===== 顶部标题栏 ===== */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <nav className="mb-1.5 flex items-center gap-2 text-sm text-muted-foreground">
            <Link to="/hosts" className="hover:text-foreground transition-colors">主机管理</Link>
            <span className="text-border">/</span>
            <span className="font-medium text-foreground">{displayName}</span>
          </nav>
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-bold tracking-tight">{displayName}</h1>
            <span className={`inline-flex items-center gap-1.5 rounded-full border ${sc.border} ${sc.bg} px-2.5 py-0.5 text-xs font-semibold ${sc.color}`}>
              <span className={`relative flex h-2 w-2`}>
                {isRunning && <span className={`absolute inline-flex h-full w-full animate-ping rounded-full opacity-60 ${sc.dot}`} />}
                <span className={`relative inline-flex h-2 w-2 rounded-full ${sc.dot}`} />
              </span>
              {sc.label}
            </span>
          </div>
          <div className="mt-1.5 flex items-center gap-3 text-sm text-muted-foreground">
            <span className="font-mono text-xs">{host.id.slice(0, 12)}…</span>
            <span className="h-3 w-px bg-border" />
            <span className="text-xs">{host.template_image_ref}</span>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {/* 主操作 */}
          {isRunning ? (
            <Button size="sm" className="h-9 gap-2 bg-primary text-primary-foreground hover:bg-primary/90" onClick={openVNC}>
              <Monitor className="h-4 w-4" /> VNC 桌面
            </Button>
          ) : null}
          {isRunning ? (
            <Button size="sm" variant="outline" className="h-9 gap-2" onClick={() => handleAction("stop")} disabled={actionMutation.isPending}>
              <Square className="h-3.5 w-3.5 fill-current" /> 停止
            </Button>
          ) : (
            <Button size="sm" className="h-9 gap-2 bg-primary text-primary-foreground hover:bg-primary/90" onClick={() => handleAction("start")} disabled={actionMutation.isPending}>
              <Play className="h-3.5 w-3.5 fill-current" /> 启动
            </Button>
          )}

          {/* 分隔 */}
          <span className="hidden h-6 w-px bg-border sm:inline-block" />

          {/* 次要操作 */}
          <Button size="sm" variant="outline" className="h-9 gap-2" onClick={() => setRebuildOpen(true)} disabled={actionMutation.isPending}>
            <RefreshCw className="h-3.5 w-3.5" /> 重建
          </Button>
          <Button
            size="sm"
            className={`h-9 gap-2 ${imageInfo?.update_available ? "bg-emerald-600 hover:bg-emerald-700 text-white" : ""}`}
            variant={imageInfo?.update_available ? "default" : "outline"}
            onClick={() => setUpgradeOpen(true)}
            disabled={actionMutation.isPending}
          >
            <ArrowUpCircle className="h-3.5 w-3.5" />
            升级
            {imageInfo?.update_available && (
              <span className="ml-1 rounded-full bg-white/20 px-1.5 py-0 text-[10px]">
                {imageInfo.latest_image_id}
              </span>
            )}
          </Button>
          <Button size="sm" variant="outline" className="h-9 gap-2" onClick={() => setRotateLoginOpen(true)}>
            <KeyRound className="h-3.5 w-3.5" /> 轮换密码
          </Button>
          <Button
            size="sm"
            variant="outline"
            className="h-9 gap-2"
            onClick={() => exportConfigMutation.mutate(hostId)}
            disabled={exportConfigMutation.isPending}
          >
            <Download className="h-3.5 w-3.5" />
            {exportConfigMutation.isPending ? "导出中..." : "导出配置"}
          </Button>
          <Button
            size="sm"
            variant="outline"
            className="h-9 gap-2"
            onClick={() => fileInputRef.current?.click()}
            disabled={importConfigMutation.isPending}
          >
            <Upload className="h-3.5 w-3.5" />
            {importConfigMutation.isPending ? "导入中..." : "导入配置"}
          </Button>

          {/* 危险操作 */}
          <Button size="sm" variant="ghost" className="h-9 w-9 p-0 text-muted-foreground hover:text-destructive" onClick={() => isRunning ? setForceDeleteOpen(true) : setDeleteOpen(true)} disabled={deleteMutation.isPending}>
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* ===== 连接卡片（页面核心 CTA） ===== */}
      {data.connection_info && (
        <Card className="overflow-hidden rounded-xl border-border/60 shadow-sm">
          <CardHeader className="flex flex-row items-center justify-between border-b border-border/40 px-6 py-4">
            <CardTitle className="flex items-center gap-2 text-base font-semibold">
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary/10">
                <Terminal className="h-4 w-4 text-primary" />
              </div>
              连接到主机
            </CardTitle>
            <div className="flex rounded-lg bg-muted p-0.5">
              {connTabs.map((t) => (
                <button
                  key={t.key}
                  onClick={() => setActiveTab(t.key)}
                  className={`rounded-md px-3.5 py-1.5 text-xs font-medium transition-all ${
                    activeTab === t.key
                      ? "bg-primary text-primary-foreground shadow-sm"
                      : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  {t.label}
                </button>
              ))}
            </div>
          </CardHeader>
          <CardContent className="space-y-4 p-6">
            <div className="relative rounded-xl bg-slate-950 px-5 py-5">
              <code className="block break-all pr-24 font-mono text-sm leading-relaxed text-slate-100">
                {activeConn.value}
              </code>
              <div className="absolute top-1/2 right-5 -translate-y-1/2">
                <CopyButton value={activeConn.value ?? ""} />
              </div>
            </div>

            {activeTab === "ssh" && sshPort && (
              <div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-xs text-muted-foreground">
                <div className="flex items-center gap-1.5">
                  <Terminal className="h-3.5 w-3.5" />
                  <span>SSH 端口</span>
                  <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-foreground">{sshPort}</span>
                </div>
                <div className="flex items-center gap-1.5">
                  <Globe className="h-3.5 w-3.5" />
                  <span>用户名</span>
                  <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-foreground">{user.username}</span>
                </div>
              </div>
            )}

            {activeTab === "curl" && (
              <p className="text-xs text-muted-foreground">
                在本地终端运行上述命令，即可自动连接到此主机的 SSH 会话。
              </p>
            )}

            {activeTab === "vnc" && (
              <Button size="sm" className="h-8 gap-1.5 bg-primary text-primary-foreground hover:bg-primary/90" onClick={openVNC}>
                <ExternalLink className="h-3.5 w-3.5" /> 打开 VNC 桌面
              </Button>
            )}
          </CardContent>
        </Card>
      )}

      {/* ===== 关键指标（Dashboard KPI 风格） ===== */}
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <StatCard
          icon={<Globe className="h-4 w-4 text-primary" />}
          label="出口 IP"
          value={egressIP || "未绑定"}
          mono
        />
        <StatCard
          icon={<Terminal className="h-4 w-4 text-primary" />}
          label="SSH 端口"
          value={sshPort ? String(sshPort) : "—"}
          mono
        />
        <StatCard
          icon={<HardDrive className="h-4 w-4 text-primary" />}
          label="镜像版本"
          value={imageInfo?.latest_image_id?.slice(0, 12) || host.template_image_ref}
          mono
          suffix={imageInfo?.update_available ? (
            <span className="ml-1.5 inline-flex items-center rounded-full bg-amber-50 px-1.5 py-0.5 text-[10px] font-medium text-amber-700">
              有更新
            </span>
          ) : null}
        />
        <StatCard
          icon={<Clock className="h-4 w-4 text-primary" />}
          label="已创建"
          value={timeSince(host.created_at)}
        />
      </div>

      {/* ===== 容器日志 ===== */}
      <HostLogsBlock hostId={host.id} />

      {/* ===== 配置详情 ===== */}
      <div className="rounded-xl border border-border/60 bg-card shadow-sm">
        <button
          onClick={() => setConfigOpen(!configOpen)}
          className="flex w-full items-center justify-between px-6 py-4 text-left transition-colors hover:bg-muted/30"
        >
          <div className="flex items-center gap-3">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary/10">
              <Settings className="h-4 w-4 text-primary" />
            </div>
            <div>
              <span className="block text-sm font-semibold">配置详情</span>
              <span className="text-xs text-muted-foreground">出口 IP 绑定、挂载路径、资源限制</span>
            </div>
          </div>
          <ChevronDown className={`h-4 w-4 text-muted-foreground transition-transform ${configOpen ? "" : "-rotate-90"}`} />
        </button>
        {configOpen && (
          <div className="border-t border-border/40 px-6 py-5">
            <div className="grid grid-cols-1 lg:grid-cols-3">
              <div className="px-2 py-2 lg:border-r lg:border-border/40 lg:px-4">
                <h3 className="mb-1 text-sm font-semibold">出口 IP 绑定</h3>
                <p className="mb-4 text-xs text-muted-foreground">每台主机必须绑定一个出口 IP</p>
                <BindingManager hostId={hostId} hostStatus={host.status} bindings={bindings} />
              </div>
              <div className="px-2 py-2 lg:px-4">
                <h3 className="mb-1 text-sm font-semibold">挂载路径</h3>
                <p className="mb-4 text-xs text-muted-foreground">{isRunning ? "运行中不可编辑" : "停止中，可以编辑"}</p>
                <MountManager hostId={hostId} hostStatus={host.status} mounts={host.host_mounts ?? []} />
              </div>
              <div className="px-2 py-2 lg:px-4">
                <h3 className="mb-1 text-sm font-semibold">资源限制</h3>
                <p className="mb-4 text-xs text-muted-foreground">
                  运行中编辑会立即应用到容器
                </p>

                {!editingResources ? (
                  <div className="space-y-2">
                    <div className="flex items-center justify-between rounded-md border bg-muted/30 px-3 py-2">
                      <span className="text-sm">进程数</span>
                      <span className="font-mono text-sm font-medium">
                        {host.pids_limit != null
                          ? host.pids_limit === 0
                            ? "默认 (1024)"
                            : String(host.pids_limit)
                          : "默认 (1024)"}
                      </span>
                    </div>
                    <div className="flex items-center justify-between rounded-md border bg-muted/30 px-3 py-2">
                      <span className="text-sm">内存</span>
                      <span className="font-mono text-sm font-medium">
                        {host.memory_limit_mb != null
                          ? host.memory_limit_mb === 0
                            ? "默认 (4 GB)"
                            : host.memory_limit_mb >= 1024
                              ? `${host.memory_limit_mb / 1024} GB`
                              : `${host.memory_limit_mb} MB`
                          : "默认 (4 GB)"}
                      </span>
                    </div>
                    <div className="flex items-center justify-between rounded-md border bg-muted/30 px-3 py-2">
                      <span className="text-sm">CPU</span>
                      <span className="font-mono text-sm font-medium">
                        {host.cpu_limit != null
                          ? host.cpu_limit === 0
                            ? "默认 (2 核)"
                            : `${host.cpu_limit} 核`
                          : "默认 (2 核)"}
                      </span>
                    </div>

                    <Button
                      size="sm"
                      variant="outline"
                      className="mt-2 w-full"
                      onClick={() => {
                        setEditResourcesValue({
                          pids_limit: host.pids_limit,
                          memory_limit_mb: host.memory_limit_mb,
                          cpu_limit: host.cpu_limit,
                        });
                        setEditingResources(true);
                      }}
                    >
                      编辑
                    </Button>
                  </div>
                ) : (
                  <div className="space-y-3">
                    <ResourceLimitsSelector
                      value={editResourcesValue}
                      onChange={setEditResourcesValue}
                    />
                    <div className="flex gap-2">
                      <Button
                        size="sm"
                        variant="outline"
                        className="flex-1"
                        onClick={() => setEditingResources(false)}
                      >
                        取消
                      </Button>
                      <Button
                        size="sm"
                        className="flex-1"
                        disabled={patchResourcesMutation.isPending}
                        onClick={() => {
                          patchResourcesMutation.mutate(
                            {
                              pids_limit: editResourcesValue.pids_limit,
                              memory_limit_mb: editResourcesValue.memory_limit_mb,
                              cpu_limit: editResourcesValue.cpu_limit,
                            },
                            {
                              onSuccess: () => setEditingResources(false),
                            },
                          );
                        }}
                      >
                        {patchResourcesMutation.isPending ? "保存中..." : "保存"}
                      </Button>
                    </div>
                  </div>
                )}
              </div>
            </div>
          </div>
        )}
      </div>

      {/* ===== Claude 状态 ===== */}
      <ClaudeStatusCard hostId={hostId} hostStatus={host.status} />

      {/* ===== 文件浏览 ===== */}
      <div className="rounded-xl border border-border/60 bg-card shadow-sm">
        <button
          type="button"
          onClick={() => setFilesOpen(!filesOpen)}
          className="flex w-full items-center justify-between px-6 py-4 text-left transition-colors hover:bg-muted/30"
        >
          <div className="flex items-center gap-3">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary/10">
              <FolderOpen className="h-4 w-4 text-primary" />
            </div>
            <div>
              <span className="block text-sm font-semibold">文件浏览</span>
              <span className="text-xs text-muted-foreground">浏览容器内文件系统</span>
            </div>
          </div>
          <ChevronDown className={`h-4 w-4 text-muted-foreground transition-transform ${filesOpen ? "" : "-rotate-90"}`} />
        </button>
        {filesOpen && (
          <div className="border-t border-border/40 px-6 py-5">
            <HostFilesBrowser hostId={hostId} />
          </div>
        )}
      </div>

      {/* ===== 代理白名单（BYPASS-UI-01） ===== */}
      <div
        id="bypass"
        className="rounded-xl border border-border/60 bg-card p-6 shadow-sm"
      >
        <BypassTab hostId={hostId} />
      </div>

      {/* ===== Dialogs ===== */}
      <RotatePasswordDialog userId={user.id} open={rotateLoginOpen} onOpenChange={setRotateLoginOpen} />
      <ChangeRootPasswordDialog hostId={host.id} open={changeRootPwOpen} onOpenChange={setChangeRootPwOpen} />
      <ClaudeSettingsDialog hostId={host.id} hostStatus={host.status} open={claudeSettingsOpen} onOpenChange={setClaudeSettingsOpen} />
      <RebuildDialog hostId={hostId} open={rebuildOpen} onOpenChange={setRebuildOpen} />

      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认删除主机？</AlertDialogTitle>
            <AlertDialogDescription>将停止并移除容器，删除数据库记录和出口 IP 绑定。不可撤销。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction className="bg-destructive text-destructive-foreground hover:bg-destructive/90" onClick={() => handleDelete(false)}>确认删除</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={forceDeleteOpen} onOpenChange={setForceDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle className="text-destructive flex items-center gap-2">
              <Trash2 className="h-5 w-5" /> 强制删除运行中的主机？
            </AlertDialogTitle>
            <AlertDialogDescription>主机正在运行，强制删除将立即终止容器并清除数据。不可撤销。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction className="bg-destructive text-destructive-foreground hover:bg-destructive/90" onClick={() => handleDelete(true)}>强制删除</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Dialog
        open={upgradeOpen}
        onOpenChange={(open) => {
          setUpgradeOpen(open);
          if (!open) {
            setUpgradeTaskId(null);
            setProgressLayers({});
            setShowLayers(false);
          }
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>
              <span className="flex items-center gap-2">
                <ArrowUpCircle className="h-5 w-5 text-emerald-600" />
                升级镜像版本
              </span>
            </DialogTitle>
          </DialogHeader>

          {!upgradeTaskId ? (
            <div className="space-y-4">
              <p className="text-sm text-muted-foreground">
                将使用最新镜像重建主机。系统会自动拉取最新镜像后重建容器。
              </p>
              {imageInfo && (
                <div className="rounded-md border bg-muted/50 p-3 text-xs space-y-1">
                  <p><strong>当前镜像：</strong><code>{imageInfo.container_image_id}</code></p>
                  <p><strong>最新镜像：</strong><code>{imageInfo.latest_image_id || imageInfo.container_image_id}</code></p>
                </div>
              )}
              <p className="text-sm text-muted-foreground">
                升级会保留 home 目录数据，仅重置系统层。
              </p>
              <DialogFooter>
                <Button variant="outline" onClick={() => setUpgradeOpen(false)}>
                  取消
                </Button>
                <Button
                  className="bg-emerald-600 text-white hover:bg-emerald-700"
                  onClick={() => {
                    actionMutation.mutate(
                      { hostId, action: "rebuild", body: { mode: "preserve" } },
                      {
                        onSuccess: (data: any) => {
                          setUpgradeTaskId(data?.task_id ?? null);
                          toast.success("升级已启动，系统正在拉取最新镜像并重建");
                        },
                        onError: () => toast.error("升级操作提交失败"),
                      },
                    );
                  }}
                  disabled={actionMutation.isPending}
                >
                  {actionMutation.isPending ? "提交中..." : "确认升级"}
                </Button>
              </DialogFooter>
            </div>
          ) : (
            <div className="space-y-4 py-2">
              <div className="flex items-center gap-3">
                {taskStatus === "succeeded" ? (
                  <CheckCircle2 className="h-5 w-5 text-emerald-600 shrink-0" />
                ) : taskStatus === "failed" ? (
                  <XCircle className="h-5 w-5 text-destructive shrink-0" />
                ) : (
                  <Loader2 className="h-5 w-5 animate-spin text-primary shrink-0" />
                )}
                <div className="flex-1 min-w-0">
                  <p className={`font-medium text-sm ${
                    taskStatus === "succeeded"
                      ? "text-emerald-600"
                      : taskStatus === "failed"
                      ? "text-destructive"
                      : "text-foreground"
                  }`}>
                    {taskStatus === "succeeded"
                      ? "升级完成"
                      : taskStatus === "failed"
                      ? "升级失败"
                      : "升级中..."}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    任务 {upgradeTaskId.slice(0, 8)}...
                  </p>
                </div>
              </div>

              {isUpgradeRunning && (task?.progress_percent ?? 0) > 0 && (
                <div className="space-y-1.5">
                  <div className="flex items-center justify-between text-xs">
                    <span className="text-muted-foreground">
                      {task?.progress_message || "处理中..."}
                    </span>
                    <span className="font-mono text-muted-foreground">
                      {task?.progress_percent}%
                    </span>
                  </div>
                  <div className="h-2 w-full overflow-hidden rounded-full bg-muted">
                    <div
                      className="h-full rounded-full bg-primary transition-all duration-500 ease-out"
                      style={{ width: `${task?.progress_percent}%` }}
                    />
                  </div>
                </div>
              )}

              {Object.keys(progressLayers).length > 0 && (
                <div className="space-y-1">
                  <button
                    type="button"
                    onClick={() => setShowLayers((s) => !s)}
                    className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
                  >
                    {showLayers ? (
                      <ChevronUp className="h-3 w-3" />
                    ) : (
                      <ChevronDown className="h-3 w-3" />
                    )}
                    层详情 ({Object.keys(progressLayers).length} 层)
                  </button>
                  {showLayers && (
                    <div className="max-h-40 overflow-auto rounded-md border bg-muted/30 p-2 space-y-1">
                      {Object.entries(progressLayers).map(([layerId, status]) => (
                        <div
                          key={layerId}
                          className="flex items-center gap-2 text-xs"
                        >
                          <code className="font-mono text-[10px] text-muted-foreground shrink-0">
                            {layerId.slice(0, 12)}
                          </code>
                          <span className="truncate text-muted-foreground">
                            {status}
                          </span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )}

              {taskStatus === "failed" && task?.last_error_summary && (
                <div className="rounded-md border border-destructive/30 bg-destructive/5 p-3">
                  <p className="text-sm font-medium text-destructive">错误详情</p>
                  <p className="mt-1 break-all text-xs text-destructive/80">
                    {task.last_error_summary}
                  </p>
                </div>
              )}

              {isUpgradeDone && (
                <DialogFooter>
                  <Button
                    onClick={() => {
                      setUpgradeOpen(false);
                      setUpgradeTaskId(null);
                      setProgressLayers({});
                    }}
                  >
                    {taskStatus === "succeeded" ? "完成" : "关闭"}
                  </Button>
                </DialogFooter>
              )}
            </div>
          )}
        </DialogContent>
      </Dialog>

      <input
        ref={fileInputRef}
        type="file"
        accept=".tar.gz,application/gzip"
        className="hidden"
        onChange={(e) => {
          const file = e.target.files?.[0];
          if (!file) return;
          importConfigMutation.mutate(
            { hostId: host.id, file },
            { onError: (err: Error) => toast.error(err.message || "导入失败") }
          );
          e.target.value = "";
        }}
      />
    </div>
  );
}

/* ===== 子组件 ===== */

function CopyButton({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <Button
      size="sm"
      className="h-8 gap-1.5 bg-white/10 text-white hover:bg-white/20"
      onClick={() => {
        navigator.clipboard.writeText(value).then(() => {
          setCopied(true);
          setTimeout(() => setCopied(false), 1500);
        });
      }}
    >
      {copied ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
      {copied ? "已复制" : "复制"}
    </Button>
  );
}

function StatCard({ icon, label, value, mono, suffix }: {
  icon: React.ReactNode;
  label: string;
  value: React.ReactNode;
  mono?: boolean;
  suffix?: React.ReactNode;
}) {
  return (
    <Card className="overflow-hidden rounded-xl border-border/60 shadow-sm">
      <CardContent className="flex flex-col p-5">
        <div className="mb-3 flex items-center gap-2">
          <div className="flex h-7 w-7 items-center justify-center rounded-md bg-primary/10">
            {icon}
          </div>
          <span className="text-xs text-muted-foreground">{label}</span>
        </div>
        <div className={`text-sm font-semibold text-foreground ${mono ? "font-mono" : ""}`}>
          {value}
          {suffix}
        </div>
      </CardContent>
    </Card>
  );
}

function HostLogsBlock({ hostId }: { hostId: string }) {
  const [autoRefresh, setAutoRefresh] = useState(true);
  const { data, isLoading, refetch, isRefetching } = useHostLogs(hostId, autoRefresh ? 5000 : false);

  return (
    <Card className="overflow-hidden rounded-xl border-border/60 shadow-sm">
      <CardHeader className="flex flex-row items-center justify-between border-b border-border/40 px-5 py-3.5">
        <CardTitle className="flex items-center gap-2 text-sm font-semibold">
          <div className="flex h-7 w-7 items-center justify-center rounded-md bg-primary/10">
            <Terminal className="h-4 w-4 text-primary" />
          </div>
          容器日志
          {data?.container_name && (
            <span className="rounded bg-muted px-1.5 py-0.5 text-[11px] font-mono text-muted-foreground">
              {data.container_name}
            </span>
          )}
        </CardTitle>
        <div className="flex items-center gap-1.5">
          {data?.error && <span className="text-xs text-destructive">{data.error}</span>}
          <Button variant="ghost" size="sm" className="h-7 gap-1 text-xs" onClick={() => refetch()} disabled={isRefetching}>
            <RefreshCw className={`h-3 w-3 ${isRefetching ? "animate-spin" : ""}`} /> 刷新
          </Button>
          <Button variant={autoRefresh ? "secondary" : "ghost"} size="sm" className="h-7 gap-1 text-xs" onClick={() => setAutoRefresh(!autoRefresh)}>
            {autoRefresh ? <Pause className="h-3 w-3" /> : <Play className="h-3 w-3" />}
            {autoRefresh ? "暂停" : "自动"}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="p-0">
        {isLoading ? (
          <div className="flex h-64 items-center justify-center bg-black">
            <div className="h-5 w-5 animate-spin rounded-full border-2 border-green-500 border-t-transparent" />
          </div>
        ) : (
          <pre className="h-64 overflow-auto bg-black p-4 font-mono text-[11px] leading-relaxed text-green-400 whitespace-pre-wrap break-all">
            {data?.logs || <span className="text-muted-foreground">暂无日志</span>}
          </pre>
        )}
      </CardContent>
    </Card>
  );
}
