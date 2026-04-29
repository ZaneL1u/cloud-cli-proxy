import { useRef, useState } from "react";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  Check,
  Copy,
  Download,
  KeyRound,
  Monitor,
  Settings,
  Terminal,
  Upload,
} from "lucide-react";
import { toast } from "sonner";
import { getToken } from "@/lib/auth";
import {
  useHostDetail,
  useHostImageInfo,
  useRestartHostVNC,
  useExportHostConfig,
  useImportHostConfig,
} from "@/hooks/use-hosts";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { BindingManager } from "@/components/hosts/binding-manager";
import { MountManager } from "@/components/hosts/mount-manager";
import { HostLifecycleActions } from "@/components/hosts/host-lifecycle-actions";
import { RotatePasswordDialog } from "@/components/users/rotate-password-dialog";
import { RotateHostSSHPasswordDialog } from "@/components/hosts/rotate-host-ssh-password-dialog";
import { ChangeRootPasswordDialog } from "@/components/hosts/change-root-password-dialog";
import { ClaudeSettingsDialog } from "@/components/hosts/claude-settings-dialog";
import { ClaudeStatusCard } from "@/components/hosts/claude-status-card";

export const Route = createFileRoute("/_dashboard/hosts/$hostId")({
  component: HostDetailPage,
});

function formatDate(dateStr: string) {
  const d = new Date(dateStr);
  return d.toLocaleDateString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

const statusConfig: Record<string, { label: string; variant: "default" | "secondary" | "destructive" | "outline" }> = {
  running: { label: "运行中", variant: "default" },
  stopped: { label: "已停止", variant: "secondary" },
  pending: { label: "等待中", variant: "outline" },
  failed: { label: "失败", variant: "destructive" },
};

const dockerStatusConfig: Record<string, { label: string; variant: "default" | "secondary" | "destructive" | "outline" }> = {
  running: { label: "运行中", variant: "default" },
  exited: { label: "已停止", variant: "secondary" },
  "not found": { label: "未创建", variant: "outline" },
  created: { label: "已创建", variant: "outline" },
  restarting: { label: "重启中", variant: "outline" },
  paused: { label: "已暂停", variant: "outline" },
  dead: { label: "已死亡", variant: "destructive" },
};

function getHostStatus(host: { status: string; docker_status?: string }) {
  const docker = host.docker_status;
  if (docker && dockerStatusConfig[docker]) return dockerStatusConfig[docker];
  return statusConfig[host.status] ?? { label: host.status, variant: "outline" as const };
}

function HostDetailPage() {
  const { hostId } = Route.useParams();
  const { data, isLoading } = useHostDetail(hostId);
  const { data: imageInfo } = useHostImageInfo(hostId, !!data?.host);
  const restartVNCMutation = useRestartHostVNC();
  const exportConfigMutation = useExportHostConfig();
  const importConfigMutation = useImportHostConfig();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [rotateLoginOpen, setRotateLoginOpen] = useState(false);
  const [rotateSSHOpen, setRotateSSHOpen] = useState(false);
  const [changeRootPwOpen, setChangeRootPwOpen] = useState(false);
  const [claudeSettingsOpen, setClaudeSettingsOpen] = useState(false);

  if (isLoading) {
    return (
      <div className="space-y-4">
        <div className="h-8 w-48 animate-pulse rounded bg-muted" />
        <div className="h-40 animate-pulse rounded bg-muted" />
      </div>
    );
  }

  if (!data) {
    return (
      <div className="py-12 text-center text-muted-foreground">
        主机不存在
      </div>
    );
  }

  const { host, user, bindings } = data;
  const sc = getHostStatus(host);

  function openVNC() {
    const token = getToken();
    const wsPath = encodeURIComponent(`v1/admin/hosts/${host.id}/vnc/`);
    window.open(
      `/v1/admin/hosts/${host.id}/vnc/vnc.html?autoconnect=true&resize=remote&path=${wsPath}&token=${token}`,
      "_blank",
    );
  }

  function restartVNC() {
    restartVNCMutation.mutate(host.id, {
      onSuccess: () => toast.success("VNC 服务已重启"),
      onError: () => toast.error("重启 VNC 失败，请稍后重试"),
    });
  }

  const displayName = host.hostname || user.username || host.id.slice(0, 8) + "…";

  return (
    <div className="space-y-6">
      <nav aria-label="面包屑" className="text-sm text-muted-foreground">
        <Link to="/hosts" className="hover:text-foreground">
          主机管理
        </Link>
        <span className="mx-2 text-border">/</span>
        <span className="font-medium text-foreground">{displayName}</span>
      </nav>

      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-3">
            <h1 className="text-2xl font-bold tracking-tight">{displayName}</h1>
            <Badge variant={sc.variant}>{sc.label}</Badge>
          </div>
          <p className="text-sm text-muted-foreground">
            所属用户{" "}
            <Link
              to="/users/$userId"
              params={{ userId: user.id }}
              className="font-medium text-primary hover:underline"
            >
              {user.username}
            </Link>
          </p>
        </div>
      </div>

      <div className="rounded-xl border border-border/80 bg-card shadow-sm">
        <div className="border-b border-border/60 px-6 py-4">
          <h2 className="text-sm font-semibold">基本信息</h2>
        </div>
        <div className="grid gap-0 md:grid-cols-2">
          <div className="border-border/60 p-6 md:border-r">
            <h3 className="mb-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              标识与归属
            </h3>
            <dl className="grid gap-3 text-sm">
              <div className="space-y-1">
                <dt className="text-xs text-muted-foreground">主机 ID</dt>
                <dd className="break-all font-mono text-sm">{host.id}</dd>
              </div>
              <div className="space-y-1">
                <dt className="text-xs text-muted-foreground">主机名</dt>
                <dd className="text-sm">{host.hostname || "—"}</dd>
              </div>
              <div className="space-y-1">
                <dt className="text-xs text-muted-foreground">所属用户</dt>
                <dd>
                  <Link
                    to="/users/$userId"
                    params={{ userId: user.id }}
                    className="text-sm text-primary hover:underline"
                  >
                    {user.username}
                  </Link>
                </dd>
              </div>
              <div className="space-y-1">
                <dt className="text-xs text-muted-foreground">Slot Key</dt>
                <dd className="text-sm">{host.slot_key}</dd>
              </div>
            </dl>
          </div>
          <div className="p-6">
            <h3 className="mb-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              配置
            </h3>
            <dl className="grid gap-3 text-sm">
              <div className="space-y-1">
                <dt className="text-xs text-muted-foreground">镜像模板</dt>
                <dd className="break-all font-mono text-xs">
                  {host.template_image_ref}
                </dd>
              </div>
              {imageInfo?.container_available && (
                <>
                  <div className="space-y-1">
                    <dt className="text-xs text-muted-foreground">当前镜像 ID</dt>
                    <dd className="font-mono text-xs">{imageInfo.container_image_id || "—"}</dd>
                  </div>
                  {imageInfo.latest_image_id && (
                    <div className="space-y-1">
                      <dt className="text-xs text-muted-foreground">最新镜像 ID</dt>
                      <dd className="font-mono text-xs">
                        {imageInfo.latest_image_id}
                        {imageInfo.update_available && (
                          <Badge variant="destructive" className="ml-2 text-[10px] px-1.5 py-0">
                            有更新
                          </Badge>
                        )}
                        {!imageInfo.update_available && (
                          <Badge variant="outline" className="ml-2 text-[10px] px-1.5 py-0 text-emerald-600 border-emerald-300">
                            已最新
                          </Badge>
                        )}
                      </dd>
                    </div>
                  )}
                </>
              )}
              <div className="space-y-1">
                <dt className="text-xs text-muted-foreground">时区</dt>
                <dd className="text-sm">{host.timezone || "—"}</dd>
              </div>
            </dl>
            <h3 className="mb-3 mt-6 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              时间
            </h3>
            <dl className="grid gap-3 text-sm">
              <div className="space-y-1">
                <dt className="text-xs text-muted-foreground">创建时间</dt>
                <dd>{formatDate(host.created_at)}</dd>
              </div>
              <div className="space-y-1">
                <dt className="text-xs text-muted-foreground">更新时间</dt>
                <dd>{formatDate(host.updated_at)}</dd>
              </div>
            </dl>
          </div>
        </div>
      </div>

      {data.connection_info && (
        <Card className="overflow-hidden rounded-xl border-border/80 shadow-sm">
          <CardHeader className="border-b bg-muted/30">
            <CardTitle className="flex items-center gap-2 text-base">
              <Terminal className="h-5 w-5" />
              连接方式
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            <div className="divide-y divide-border/60">
              <ConnectionBlock
                label="一键连接（curl 入口）"
                command={data.connection_info.curl_command}
              />
              <div className="p-6 space-y-3">
                <ConnectionBlock
                  label="SSH 直连（配置入站密钥后可免密登录）"
                  command={data.connection_info.ssh_command}
                  inline
                />
                <Button
                  type="button"
                  variant="default"
                  className="h-10 gap-2"
                  onClick={() => {
                    navigator.clipboard.writeText(data.connection_info!.ssh_command);
                    toast.success("SSH 命令已复制，请在终端中粘贴执行");
                  }}
                >
                  <Terminal className="h-4 w-4" />
                  复制 SSH 命令
                </Button>
              </div>
              {data.connection_info.vnc_url && (
                <div className="space-y-4 p-6">
                  <ConnectionBlock
                    label="VNC 登录入口"
                    command={data.connection_info.vnc_url}
                    inline
                  />
                  <Button
                    type="button"
                    variant="outline"
                    className="h-10 gap-2"
                    onClick={openVNC}
                  >
                    <Monitor className="h-4 w-4" />
                    打开浏览器桌面
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    className="h-10 gap-2"
                    disabled={restartVNCMutation.isPending}
                    onClick={restartVNC}
                  >
                    <Monitor className="h-4 w-4" />
                    {restartVNCMutation.isPending ? "重启中..." : "重启 VNC 服务"}
                  </Button>
                </div>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2 lg:items-start">
        <Card className="rounded-xl border-border/80 shadow-sm">
          <CardHeader className="border-b bg-muted/30 pb-4">
            <CardTitle className="text-base">出口 IP 绑定</CardTitle>
            <CardDescription className="text-xs leading-relaxed">
              未运行时可增删绑定；运行中主机需先停止后再调整。
            </CardDescription>
          </CardHeader>
          <CardContent className="p-6 pt-5">
            <BindingManager
              hostId={hostId}
              hostStatus={host.status}
              bindings={bindings}
            />
          </CardContent>
        </Card>

        <Card className="rounded-xl border-border/80 shadow-sm">
          <CardHeader className="border-b bg-muted/30 pb-4">
            <CardTitle className="text-base">挂载路径</CardTitle>
            <CardDescription className="text-xs leading-relaxed">
              配置宿主机目录到容器的 bind mount。运行中主机需先停止后再编辑。
            </CardDescription>
          </CardHeader>
          <CardContent className="p-6 pt-5">
            <MountManager
              hostId={hostId}
              hostStatus={host.status}
              mounts={host.host_mounts ?? []}
            />
          </CardContent>
        </Card>

        <Card className="rounded-xl border-border/80 shadow-sm">
          <CardHeader className="border-b bg-muted/30 pb-4">
            <CardTitle className="text-base">生命周期与密码操作</CardTitle>
            <CardDescription className="text-xs leading-relaxed">
              电源与重建会生成任务；密码与桌面操作异步执行。
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-0 p-0">
            <div className="p-6 pt-5">
              <HostLifecycleActions hostId={hostId} hostStatus={host.status} imageInfo={imageInfo} />
            </div>
            <Separator />
            <div className="space-y-4 bg-muted/25 p-6">
              <div>
                <p className="mb-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  凭据与远程
                </p>
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <Button
                    type="button"
                    variant="secondary"
                    className="h-11 justify-start gap-2 px-4 sm:col-span-1"
                    onClick={() => setRotateSSHOpen(true)}
                  >
                    <KeyRound className="h-4 w-4 shrink-0" />
                    <span className="text-left text-sm leading-snug">
                      重置主机 SSH 密码
                    </span>
                  </Button>
                  <Button
                    type="button"
                    variant="secondary"
                    className="h-11 justify-start gap-2 px-4"
                    onClick={() => setRotateLoginOpen(true)}
                  >
                    <KeyRound className="h-4 w-4 shrink-0" />
                    <span className="text-left text-sm leading-snug">
                      轮换用户登录密码
                    </span>
                  </Button>
                  <Button
                    type="button"
                    variant="secondary"
                    className="h-11 justify-start gap-2 px-4 sm:col-span-1"
                    onClick={() => setChangeRootPwOpen(true)}
                  >
                    <KeyRound className="h-4 w-4 shrink-0" />
                    <span className="text-left text-sm leading-snug">
                      修改 Root 密码
                    </span>
                  </Button>
                  <Button
                    type="button"
                    variant="secondary"
                    className="h-11 justify-start gap-2 px-4 sm:col-span-1"
                    onClick={() => setClaudeSettingsOpen(true)}
                  >
                    <Settings className="h-4 w-4 shrink-0" />
                    <span className="text-left text-sm leading-snug">
                      编辑 Claude 配置
                    </span>
                  </Button>
                  <Button
                    type="button"
                    variant="secondary"
                    className="h-11 justify-start gap-2 px-4 sm:col-span-1"
                    disabled={host.status !== "running" || exportConfigMutation.isPending}
                    onClick={() =>
                      exportConfigMutation.mutate(host.id, {
                        onError: (err: Error) => toast.error(err.message || "导出配置失败"),
                      })
                    }
                  >
                    <Download className="h-4 w-4 shrink-0" />
                    <span className="text-left text-sm leading-snug">
                      {exportConfigMutation.isPending ? "导出中..." : "导出配置"}
                    </span>
                  </Button>
                  <Button
                    type="button"
                    variant="secondary"
                    className="h-11 justify-start gap-2 px-4 sm:col-span-1"
                    disabled={host.status !== "running" || importConfigMutation.isPending}
                    onClick={() => fileInputRef.current?.click()}
                  >
                    <Upload className="h-4 w-4 shrink-0" />
                    <span className="text-left text-sm leading-snug">
                      {importConfigMutation.isPending ? "导入中..." : "导入配置"}
                    </span>
                  </Button>
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
                        {
                          onError: (err: Error) => toast.error(err.message || "导入配置失败"),
                        }
                      );
                      e.target.value = "";
                    }}
                  />
                  <Button
                    type="button"
                    variant="secondary"
                    className="h-11 justify-start gap-2 px-4 sm:col-span-2"
                    onClick={openVNC}
                  >
                    <Monitor className="h-4 w-4 shrink-0" />
                    <span className="text-left text-sm leading-snug">
                      打开 VNC 桌面
                    </span>
                  </Button>
                  <Button
                    type="button"
                    variant="secondary"
                    className="h-11 justify-start gap-2 px-4 sm:col-span-2"
                    disabled={restartVNCMutation.isPending}
                    onClick={restartVNC}
                  >
                    <Monitor className="h-4 w-4 shrink-0" />
                    <span className="text-left text-sm leading-snug">
                      {restartVNCMutation.isPending ? "重启 VNC 中..." : "重启 VNC 服务"}
                    </span>
                  </Button>
                </div>
              </div>
              <p className="text-xs leading-relaxed text-muted-foreground">
                操作提交后将异步执行，请在「任务列表」中查看进度。
              </p>
            </div>
          </CardContent>
        </Card>
      </div>

      <ClaudeStatusCard hostId={hostId} hostStatus={host.status} />

      <RotatePasswordDialog
        userId={user.id}
        open={rotateLoginOpen}
        onOpenChange={setRotateLoginOpen}
      />
      <RotateHostSSHPasswordDialog
        hostId={host.id}
        open={rotateSSHOpen}
        onOpenChange={setRotateSSHOpen}
      />
      <ChangeRootPasswordDialog
        hostId={host.id}
        open={changeRootPwOpen}
        onOpenChange={setChangeRootPwOpen}
      />
      <ClaudeSettingsDialog
        hostId={host.id}
        hostStatus={host.status}
        open={claudeSettingsOpen}
        onOpenChange={setClaudeSettingsOpen}
      />
    </div>
  );
}

function ConnectionBlock({
  label,
  command,
  inline,
}: {
  label: string;
  command: string;
  inline?: boolean;
}) {
  const [copied, setCopied] = useState(false);

  function handleCopy() {
    navigator.clipboard.writeText(command).then(() => {
      setCopied(true);
      toast.success("已复制到剪贴板");
      setTimeout(() => setCopied(false), 2000);
    });
  }

  const content = (
    <div className="space-y-2">
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

  if (inline) return content;
  return <div className="p-6">{content}</div>;
}
