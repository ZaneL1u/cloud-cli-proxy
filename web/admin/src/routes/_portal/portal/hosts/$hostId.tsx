import { createFileRoute, Link } from "@tanstack/react-router";
import { ArrowLeft, Globe, Shield, Monitor, Terminal, Copy, Check } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { getToken } from "@/lib/auth";
import {
  useMyHostDetail,
  useRebuildHost,
} from "@/hooks/use-portal-hosts";
import {
  useMySSHKeys,
  useMyGenerateSSHKey,
  useMySetSSHKey,
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

const tunnelTypeLabels: Record<string, string> = {
  wireguard: "WireGuard",
  proxy: "代理隧道",
};

function PortalHostDetail() {
  const { hostId } = Route.useParams();
  const rebuildMutation = useRebuildHost();
  const sshKeysQuery = useMySSHKeys();
  const generateSSHKey = useMyGenerateSSHKey();
  const setSSHKey = useMySetSSHKey();

  const isRebuilding = (status: string) =>
    status === "rebuilding" || status === "pending";

  const { data: host, isLoading } = useMyHostDetail(hostId, {
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status && isRebuilding(status) ? 3000 : false;
    },
  });

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
        <Link
          to="/portal"
          className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          返回主机列表
        </Link>
        <p className="text-muted-foreground">主机未找到</p>
      </div>
    );
  }

  const sc = statusConfig[host.status] ?? {
    label: host.status,
    variant: "outline" as const,
  };

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

  return (
    <div className="space-y-6">
      {/* Breadcrumb */}
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Link
          to="/portal"
          className="inline-flex items-center gap-1 hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          我的主机
        </Link>
        <span>/</span>
        <span className="text-foreground">{host.hostname || "主机详情"}</span>
      </div>

      {/* Basic info */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0">
          <CardTitle className="text-xl">{host.hostname || "未命名主机"}</CardTitle>
          <Badge variant={sc.variant}>{sc.label}</Badge>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <dt className="text-sm font-medium text-muted-foreground">时区</dt>
              <dd className="mt-1 text-sm">{host.timezone || "未设置"}</dd>
            </div>
            <div>
              <dt className="text-sm font-medium text-muted-foreground">创建时间</dt>
              <dd className="mt-1 text-sm">{formatDateTime(host.created_at)}</dd>
            </div>
            <div>
              <dt className="text-sm font-medium text-muted-foreground">更新时间</dt>
              <dd className="mt-1 text-sm">{formatDateTime(host.updated_at)}</dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      {/* Egress bindings */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">出口 IP</CardTitle>
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
                      {tunnelTypeLabels[binding.tunnel_type] ?? binding.tunnel_type}
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
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">快速访问</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <Button
              onClick={() => {
                const token = getToken() || "";
                const wsPath = encodeURIComponent(
                  `v1/user/hosts/${host.id}/vnc/`
                );
                window.open(
                  `/v1/user/hosts/${host.id}/vnc/vnc.html?autoconnect=true&resize=remote&path=${wsPath}&token=${token}`,
                  "_blank"
                );
              }}
              className="w-full justify-start gap-2"
              variant="outline"
            >
              <Monitor className="h-4 w-4" />
              打开桌面（VNC）
            </Button>
          </CardContent>
        </Card>
      )}

      {/* Connection Info */}
      {host.connection_info && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-lg">
              <Terminal className="h-5 w-5" />
              SSH 连接
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <p className="text-sm text-muted-foreground">
                在终端中运行以下命令，一键连接到你的云主机：
              </p>
              <CopyableCommand command={host.connection_info.curl_command} />
            </div>
            <div className="space-y-2">
              <p className="text-sm text-muted-foreground">
                或者使用 SSH 直连（需要用入口密码）：
              </p>
              <CopyableCommand command={host.connection_info.ssh_command} />
            </div>
          </CardContent>
        </Card>
      )}

      {/* SSH Keys */}
      <SSHKeyManager
        data={sshKeysQuery.data}
        isLoading={sshKeysQuery.isLoading}
        onGenerate={(keyType) =>
          generateSSHKey.mutate(keyType, {
            onSuccess: () => toast.success("SSH 密钥已生成"),
            onError: () => toast.error("生成失败"),
          })
        }
        onSet={(publicKey, privateKey) =>
          setSSHKey.mutate(
            { publicKey, privateKey },
            {
              onSuccess: () => toast.success("SSH 密钥已保存"),
              onError: () => toast.error("保存失败"),
            },
          )
        }
        isGenerating={generateSSHKey.isPending}
        isSetting={setSSHKey.isPending}
      />

      {/* Actions */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">操作</CardTitle>
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

function CopyableCommand({ command }: { command: string }) {
  const [copied, setCopied] = useState(false);

  function handleCopy() {
    navigator.clipboard.writeText(command).then(() => {
      setCopied(true);
      toast.success("已复制到剪贴板");
      setTimeout(() => setCopied(false), 2000);
    });
  }

  return (
    <div className="flex items-center gap-2 rounded-lg border bg-muted/50 p-3">
      <code className="flex-1 overflow-x-auto text-sm font-mono">
        {command}
      </code>
      <Button
        variant="ghost"
        size="icon"
        className="h-8 w-8 shrink-0"
        onClick={handleCopy}
      >
        {copied ? (
          <Check className="h-4 w-4 text-green-600" />
        ) : (
          <Copy className="h-4 w-4" />
        )}
      </Button>
    </div>
  );
}
