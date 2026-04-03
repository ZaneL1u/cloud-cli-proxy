import { useState, type FormEvent } from "react";
import { createFileRoute, Link } from "@tanstack/react-router";
import { Globe, Server } from "lucide-react";
import { toast } from "sonner";
import { useMyHosts } from "@/hooks/use-portal-hosts";
import type { PortalHost } from "@/hooks/use-portal-hosts";
import { useChangeLoginPassword } from "@/hooks/use-portal-password";
import { ApiError } from "@/lib/api";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { PageHeader } from "@/components/layout/page-header";
import { EmptyState } from "@/components/layout/empty-state";

export const Route = createFileRoute("/_portal/portal/")({
  component: PortalHostList,
});

function formatDate(dateStr: string): string {
  const d = new Date(dateStr);
  return d.toLocaleDateString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  });
}

const statusStyles: Record<string, { label: string; className: string }> = {
  running: { label: "运行中", className: "bg-green-100 text-green-700" },
  stopped: { label: "已停止", className: "bg-gray-100 text-gray-700" },
  rebuilding: { label: "重建中", className: "bg-yellow-100 text-yellow-700" },
  pending: { label: "等待中", className: "bg-blue-100 text-blue-700" },
};

const statusBarClass: Record<string, string> = {
  running: "bg-emerald-500",
  stopped: "bg-muted-foreground/35",
  rebuilding: "bg-amber-500",
  pending: "bg-sky-500",
};

function StatusBadge({ status }: { status: string }) {
  const style = statusStyles[status] ?? {
    label: status,
    className: "bg-gray-100 text-gray-700",
  };
  return (
    <span
      className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${style.className}`}
    >
      {style.label}
    </span>
  );
}

function HostCard({ host }: { host: PortalHost }) {
  const bar =
    statusBarClass[host.status] ?? "bg-muted-foreground/35";

  return (
    <Link
      to="/portal/hosts/$hostId"
      params={{ hostId: host.id }}
      className="group block"
    >
      <Card className="relative overflow-hidden rounded-xl border-border/80 shadow-sm transition-all duration-200 hover:-translate-y-1 hover:shadow-md">
        <span
          className={`absolute inset-y-0 left-0 w-1 ${bar}`}
          aria-hidden
        />
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2 pl-5">
          <CardTitle className="text-base font-semibold">
            {host.hostname || "未命名主机"}
          </CardTitle>
          <StatusBadge status={host.status} />
        </CardHeader>
        <CardContent className="space-y-2 pl-5">
          <div className="flex items-center gap-1.5 text-sm text-muted-foreground">
            <Globe className="h-4 w-4" />
            <span className="font-mono">{host.egress_ip || "未分配"}</span>
          </div>
          <div className="text-xs text-muted-foreground">
            创建于 {formatDate(host.created_at)}
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}

function parseApiError(err: unknown): string {
  if (err instanceof ApiError) {
    try {
      const j = JSON.parse(err.message) as { error?: string };
      if (j.error) return j.error;
    } catch {
      return err.message;
    }
  }
  return "请求失败";
}

function PortalHostList() {
  const { data, isLoading } = useMyHosts();
  const hosts = data?.hosts ?? [];
  const changeLogin = useChangeLoginPassword();
  const [loginOld, setLoginOld] = useState("");
  const [loginNew, setLoginNew] = useState("");
  const [loginConfirm, setLoginConfirm] = useState("");

  function submitLoginPassword(e: FormEvent) {
    e.preventDefault();
    if (loginNew.length < 8) {
      toast.error("新密码至少 8 个字符");
      return;
    }
    if (loginNew !== loginConfirm) {
      toast.error("两次输入的新密码不一致");
      return;
    }
    changeLogin.mutate(
      { old_password: loginOld, new_password: loginNew },
      {
        onSuccess: () => {
          toast.success("登录密码已更新");
          setLoginOld("");
          setLoginNew("");
          setLoginConfirm("");
        },
        onError: (err) => toast.error(parseApiError(err)),
      },
    );
  }

  if (isLoading) {
    return (
      <div className="space-y-6">
        <PageHeader title="我的主机" description="加载中…" />
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <Card key={i}>
              <CardHeader>
                <div className="h-5 w-32 animate-pulse rounded bg-muted" />
              </CardHeader>
              <CardContent className="space-y-2">
                <div className="h-4 w-24 animate-pulse rounded bg-muted" />
                <div className="h-3 w-20 animate-pulse rounded bg-muted" />
              </CardContent>
            </Card>
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <PageHeader
        title="我的主机"
        description="欢迎回来。在此查看云主机状态、出口 IP，并管理登录密码与连接说明。"
      />

      <Card className="rounded-xl border-amber-200/80 bg-amber-50/40 shadow-sm">
        <CardHeader className="border-b border-amber-200/60 bg-amber-100/40">
          <CardTitle className="text-base">重建数据保留规则（请务必阅读）</CardTitle>
          <CardDescription>
            重建主机前，请先确认你的数据是否存放在正确目录。
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-2 text-sm text-muted-foreground">
          <p>
            只有 <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">/workspace</code> 是重建保留区。
          </p>
          <p>
            以下数据默认会保留：
            <code className="ml-1 rounded bg-muted px-1 py-0.5 font-mono text-xs">/workspace/.claude</code>
            （Claude 登录与配置）、
            <code className="ml-1 rounded bg-muted px-1 py-0.5 font-mono text-xs">/workspace/.chrome-data</code>
            （浏览器会话与 Cookie）。
          </p>
          <p>
            非 <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">/workspace</code> 路径（如
            <code className="ml-1 rounded bg-muted px-1 py-0.5 font-mono text-xs">/etc</code>、
            <code className="ml-1 rounded bg-muted px-1 py-0.5 font-mono text-xs">/usr</code>、
            <code className="ml-1 rounded bg-muted px-1 py-0.5 font-mono text-xs">/var</code>、
            <code className="ml-1 rounded bg-muted px-1 py-0.5 font-mono text-xs">/tmp</code>）会被重置。
          </p>
          <p className="text-amber-800">
            若管理员执行“清空所有数据并重建”，<code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">/workspace</code> 也会被清空。
          </p>
        </CardContent>
      </Card>

      {hosts.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border/80 bg-muted/20 p-2">
          <EmptyState
            icon={Server}
            title="暂无主机"
            description="请联系管理员为您创建主机后即可在此管理"
          />
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
          {hosts.map((host) => (
            <HostCard key={host.id} host={host} />
          ))}
        </div>
      )}

      <div className="grid gap-6 md:grid-cols-2">
        <Card className="rounded-xl border-border/80 shadow-sm">
          <CardHeader className="border-b bg-muted/30">
            <CardTitle className="text-base">修改登录密码</CardTitle>
            <CardDescription>
              用于网页登录与 curl 入口，与 SSH 密码不同。
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={submitLoginPassword} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="portal-login-old">当前密码</Label>
                <Input
                  id="portal-login-old"
                  type="password"
                  autoComplete="current-password"
                  value={loginOld}
                  onChange={(e) => setLoginOld(e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="portal-login-new">新密码</Label>
                <Input
                  id="portal-login-new"
                  type="password"
                  autoComplete="new-password"
                  value={loginNew}
                  onChange={(e) => setLoginNew(e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="portal-login-confirm">确认新密码</Label>
                <Input
                  id="portal-login-confirm"
                  type="password"
                  autoComplete="new-password"
                  value={loginConfirm}
                  onChange={(e) => setLoginConfirm(e.target.value)}
                />
              </div>
              <Button
                type="submit"
                disabled={
                  changeLogin.isPending ||
                  !loginOld ||
                  !loginNew ||
                  !loginConfirm
                }
              >
                {changeLogin.isPending ? "保存中…" : "更新登录密码"}
              </Button>
            </form>
          </CardContent>
        </Card>

        <Card className="rounded-xl border-border/80 shadow-sm">
          <CardHeader className="border-b bg-muted/30">
            <CardTitle className="text-base">SSH 与桌面连接</CardTitle>
            <CardDescription>
              SSH 密码现在按主机分别管理，不再是用户级共享密码。
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="rounded-lg border border-border/60 bg-card p-4 text-sm leading-relaxed text-muted-foreground shadow-sm">
              每台主机都有独立的 SSH 短 ID 与 SSH 密码。请进入具体主机详情页查看连接命令、VNC 入口和当前主机的接入方式。
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
