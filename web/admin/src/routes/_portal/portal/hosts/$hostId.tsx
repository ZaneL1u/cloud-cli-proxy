import { createFileRoute, Link } from "@tanstack/react-router";
import {
  Check,
  Copy,
  Globe,
  Loader2,
  Monitor,
  PanelTop,
  Play,
  RefreshCw,
  Shield,
  Terminal,
} from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { getToken } from "@/lib/auth";
import {
  useMyHostDetail,
  useMyHostVNCStatus,
  useRebuildHost,
  useRestartMyHostVNC,
  useStartMyHostVNC,
} from "@/hooks/use-portal-hosts";
import {
  useMySSHKeys,
  useMyCreateSSHKey,
  useMyDeleteSSHKey,
} from "@/hooks/use-ssh-keys";
import { SSHKeyManager } from "@/components/ssh-keys/ssh-key-manager";
import { Badge } from "@/components/ui/badge";
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
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";

export const Route = createFileRoute("/_portal/portal/hosts/$hostId")({
  component: PortalHostDetail,
});

function formatDateTime(dateStr: string): string {
  const d = new Date(dateStr);
  return d.toLocaleDateString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

const statusConfig: Record<
  string,
  { label: string; variant: "default" | "secondary" | "destructive" | "outline" }
> = {
  running: { label: "运行中", variant: "default" },
  stopped: { label: "已停止", variant: "secondary" },
  rebuilding: { label: "重建中", variant: "outline" },
  pending: { label: "等待中", variant: "outline" },
};

const vncStatusConfig: Record<
  string,
  { label: string; className: string; dot: string }
> = {
  running: {
    label: "VNC 运行中",
    className: "border-emerald-200 bg-emerald-50 text-emerald-700",
    dot: "bg-emerald-500",
  },
  stopped: {
    label: "VNC 已关闭",
    className: "border-amber-200 bg-amber-50 text-amber-700",
    dot: "bg-amber-500",
  },
  host_stopped: {
    label: "主机未运行",
    className: "border-slate-200 bg-slate-50 text-slate-600",
    dot: "bg-slate-400",
  },
  unavailable: {
    label: "状态不可用",
    className: "border-red-200 bg-red-50 text-red-700",
    dot: "bg-red-500",
  },
  loading: {
    label: "检测中",
    className: "border-slate-200 bg-slate-50 text-slate-600",
    dot: "bg-slate-400",
  },
};


function PortalHostDetail() {
  const { hostId } = Route.useParams();
  const rebuildMutation = useRebuildHost();
  const startVNCMutation = useStartMyHostVNC();
  const restartVNCMutation = useRestartMyHostVNC();
  const sshKeysQuery = useMySSHKeys();
  const createSSHKey = useMyCreateSSHKey();
  const deleteSSHKey = useMyDeleteSSHKey();

  const isRebuilding = (status: string) =>
    status === "rebuilding" || status === "pending";

  const { data: host, isLoading } = useMyHostDetail(hostId, {
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status && isRebuilding(status) ? 3000 : false;
    },
  });
  const vncStatusQuery = useMyHostVNCStatus(hostId, !!host);

  if (isLoading) {
    return (
      <div className="space-y-4">
        <div className="h-6 w-32 animate-pulse rounded bg-muted" />
        <div className="h-48 animate-pulse rounded bg-muted" />
      </div>
    );
  }

  if (!host) {
    return (
      <div className="space-y-4">
        <nav className="text-sm text-muted-foreground">
          <Link to="/portal" className="hover:text-foreground">
            我的主机
          </Link>
          <span className="mx-2">/</span>
          <span className="text-foreground">未找到</span>
        </nav>
        <p className="text-muted-foreground">主机未找到</p>
      </div>
    );
  }

  const currentHost = host;
  const sc = statusConfig[currentHost.status] ?? {
    label: currentHost.status,
    variant: "outline" as const,
  };
  const vncStatus = vncStatusQuery.data;
  const vncSC = vncStatus
    ? (vncStatusConfig[vncStatus.status] || vncStatusConfig.unavailable)
    : vncStatusConfig.loading;
  const isVNCBusy = startVNCMutation.isPending || restartVNCMutation.isPending;

  function handleRebuild() {
    rebuildMutation.mutate(hostId, {
      onSuccess: () => {
        toast.success("重建任务已提交");
      },
      onError: () => {
        toast.error("重建请求失败，请稍后重试");
      },
    });
  }

  function openVNC() {
    const token = getToken() || "";
    const wsPath = encodeURIComponent(`v1/user/hosts/${currentHost.id}/vnc/`);
    window.open(
      `/v1/user/hosts/${currentHost.id}/vnc/vnc.html?autoconnect=true&resize=remote&path=${wsPath}&token=${token}`,
      "_blank"
    );
  }

  function handleStartVNC() {
    startVNCMutation.mutate(currentHost.id, {
      onSuccess: () => toast.success("VNC 服务已启动"),
      onError: () => toast.error("启动 VNC 失败，请稍后重试"),
    });
  }

  function handleRestartVNC() {
    restartVNCMutation.mutate(currentHost.id, {
      onSuccess: () => toast.success("VNC 服务已重启"),
      onError: () => toast.error("重启 VNC 失败，请稍后重试"),
    });
  }

  const displayName = currentHost.hostname || "未命名主机";

  return (
    <div className="space-y-6">
      <nav aria-label="面包屑" className="text-sm text-muted-foreground">
        <Link to="/portal" className="hover:text-foreground">
          我的主机
        </Link>
        <span className="mx-2 text-border">/</span>
        <span className="font-medium text-foreground">{displayName}</span>
      </nav>

      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-1">
          <div className="flex flex-wrap items-center gap-3">
            <h1 className="text-2xl font-bold tracking-tight">{displayName}</h1>
            <Badge variant={sc.variant}>{sc.label}</Badge>
          </div>
          <p className="text-sm text-muted-foreground">
            查看出口绑定、连接命令与运维操作
          </p>
        </div>
      </div>

      <Card className="overflow-hidden rounded-xl border-border/80 shadow-sm">
        <CardHeader className="border-b bg-muted/30">
          <CardTitle className="text-base">基本信息</CardTitle>
        </CardHeader>
        <CardContent className="pt-6">
          <dl className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <dt className="text-xs font-medium text-muted-foreground">时区</dt>
              <dd className="mt-1 text-sm">{host.timezone || "未设置"}</dd>
            </div>
            <div>
              <dt className="text-xs font-medium text-muted-foreground">创建时间</dt>
              <dd className="mt-1 text-sm">{formatDateTime(host.created_at)}</dd>
            </div>
            <div>
              <dt className="text-xs font-medium text-muted-foreground">更新时间</dt>
              <dd className="mt-1 text-sm">{formatDateTime(host.updated_at)}</dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      <Card className="overflow-hidden rounded-xl border-border/80 shadow-sm">
        <CardHeader className="border-b bg-muted/30">
          <CardTitle className="text-base">出口 IP</CardTitle>
        </CardHeader>
        <CardContent>
          {host.egress_bindings.length === 0 ? (
            <p className="text-sm text-muted-foreground">暂无绑定的出口 IP</p>
          ) : (
            <div className="space-y-3">
              {host.egress_bindings.map((binding, idx) => (
                <div
                  key={idx}
                  className="flex items-center justify-between rounded-lg border p-3"
                >
                  <div className="flex items-center gap-2">
                    <Globe className="h-4 w-4 text-muted-foreground" />
                    <span className="font-mono text-sm">{binding.ip_address}</span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Shield className="h-3.5 w-3.5 text-muted-foreground" />
                    <span className="text-sm text-muted-foreground">
                      代理隧道
                    </span>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Quick Access */}
      {host.status === "running" && (
        <Card className="overflow-hidden rounded-xl border-border/80 shadow-sm">
          <CardHeader className="border-b bg-muted/30">
            <CardTitle className="text-base">快速访问</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4 pt-6">
            <Button
              onClick={openVNC}
              className="h-auto w-full flex-col gap-2 py-5 sm:flex-row sm:justify-start"
              variant="secondary"
              disabled={!vncStatus?.running}
            >
              <div className="relative flex h-12 w-12 items-center justify-center rounded-xl bg-background shadow-sm ring-1 ring-border">
                <PanelTop className="h-6 w-6 text-muted-foreground/80" />
                <Monitor className="absolute bottom-1.5 right-1.5 h-4 w-4 text-primary" />
              </div>
              <span className="text-sm font-medium">打开浏览器桌面（VNC）</span>
            </Button>
            <div className="flex flex-col gap-3 border-t border-border/60 pt-4 sm:flex-row sm:items-center sm:justify-between">
              <div className="flex flex-wrap items-center gap-2">
                <span className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-semibold ${vncSC.className}`}>
                  <span className={`h-2 w-2 rounded-full ${vncSC.dot}`} />
                  {vncSC.label}
                </span>
                {vncStatus?.auto_restart_limited && (
                  <span className="text-xs text-amber-700">自动拉起已暂停，手动启动会重置</span>
                )}
              </div>
              <div className="flex flex-wrap gap-2">
                <Button
                  type="button"
                  variant="outline"
                  disabled={isVNCBusy || !vncStatus?.can_start}
                  onClick={handleStartVNC}
                >
                  {startVNCMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4 fill-current" />}
                  启动 VNC
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  disabled={isVNCBusy || !vncStatus?.can_restart}
                  onClick={handleRestartVNC}
                >
                  {restartVNCMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
                  重启 VNC
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Connection Info */}
      {host.connection_info && (
        <Card className="overflow-hidden rounded-xl border-border/80 shadow-sm">
          <CardHeader className="border-b bg-muted/30">
            <CardTitle className="flex items-center gap-2 text-base">
              <Terminal className="h-5 w-5" />
              SSH 连接
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            <div className="divide-y divide-border/60">
              <ConnectionBlock
                label="一键连接（curl 入口）"
                command={host.connection_info.curl_command}
              />
              <ConnectionBlock
                label="SSH 直连（需要 SSH 密码）"
                command={host.connection_info.ssh_command}
              />
            </div>
          </CardContent>
        </Card>
      )}

      {/* SSH Keys */}
      <SSHKeyManager
        keys={sshKeysQuery.data?.keys ?? []}
        isLoading={sshKeysQuery.isLoading}
        onCreate={(params) =>
          createSSHKey.mutate(params, {
            onSuccess: () => toast.success("SSH 密钥已创建"),
            onError: () => toast.error("创建失败"),
          })
        }
        onDelete={(keyId) =>
          deleteSSHKey.mutate(keyId, {
            onSuccess: () => toast.success("SSH 密钥已删除"),
            onError: () => toast.error("删除失败"),
          })
        }
        isCreating={createSSHKey.isPending}
        isDeleting={deleteSSHKey.isPending}
        lastCreatedKey={createSSHKey.data?.key}
      />

      {/* Actions */}
      <Card className="overflow-hidden rounded-xl border-border/80 shadow-sm">
        <CardHeader className="border-b bg-muted/30">
          <CardTitle className="text-base">操作</CardTitle>
        </CardHeader>
        <CardContent>
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button
                variant="outline"
                disabled={rebuildMutation.isPending || isRebuilding(host.status)}
              >
                {rebuildMutation.isPending ? "提交中..." : "重建主机"}
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>确认重建主机？</AlertDialogTitle>
                <AlertDialogDescription asChild>
                  <div className="space-y-2 text-sm text-muted-foreground">
                    <p>重建将重置容器系统环境，重建过程中主机将暂时不可访问。</p>
                    <div className="rounded-md border bg-muted/50 p-2.5 text-xs space-y-1">
                      <p><strong className="text-foreground">保留：</strong>home 目录（/workspace）下所有文件、SSH 密钥（自动重新注入）、SSH 密码</p>
                      <p><strong className="text-foreground">清除：</strong>通过 apt 安装的额外软件包、系统级配置修改、/tmp 等临时目录</p>
                    </div>
                  </div>
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>取消</AlertDialogCancel>
                <AlertDialogAction
                  disabled={rebuildMutation.isPending}
                  onClick={handleRebuild}
                >
                  {rebuildMutation.isPending ? "重建中..." : "确认重建"}
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </CardContent>
      </Card>
    </div>
  );
}

function ConnectionBlock({ label, command }: { label: string; command: string }) {
  const [copied, setCopied] = useState(false);

  function handleCopy() {
    navigator.clipboard.writeText(command).then(() => {
      setCopied(true);
      toast.success("已复制到剪贴板");
      setTimeout(() => setCopied(false), 2000);
    });
  }

  return (
    <div className="space-y-2 p-6">
      <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        {label}
      </p>
      <div className="group relative overflow-hidden rounded-lg border border-border/60 bg-muted/40 transition-colors hover:bg-muted/60">
        <code className="block break-all px-4 py-3 pr-12 font-mono text-sm leading-relaxed text-foreground">
          {command}
        </code>
        <Button
          variant="ghost"
          size="icon"
          className="absolute right-2 top-1/2 h-8 w-8 -translate-y-1/2 opacity-60 transition-opacity hover:opacity-100 group-hover:opacity-100"
          onClick={handleCopy}
        >
          {copied ? (
            <Check className="h-4 w-4 text-emerald-500" />
          ) : (
            <Copy className="h-4 w-4" />
          )}
        </Button>
      </div>
    </div>
  );
}
