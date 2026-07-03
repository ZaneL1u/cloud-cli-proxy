import { useState, type FormEvent } from "react";
import { createFileRoute, Link } from "@tanstack/react-router";
import { Globe, Server } from "lucide-react";
import { toast } from "sonner";
import { useMyHosts } from "@/hooks/use-portal-hosts";
import type { PortalHost } from "@/hooks/use-portal-hosts";
import { useChangeLoginPassword } from "@/hooks/use-portal-password";
import { ApiError } from "@/lib/api";
import { hostStatusConfig, defaultHostStatus } from "@/lib/status-constants";
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
import { Badge } from "@/components/ui/badge";
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

function StatusBadge({ status }: { status: string }) {
  const cfg = hostStatusConfig[status] ?? defaultHostStatus;
  return <Badge variant={cfg.badgeVariant}>{cfg.label}</Badge>;
}

function HostCard({ host }: { host: PortalHost }) {
  const cfg = hostStatusConfig[host.status] ?? defaultHostStatus;

  return (
    <Link
      to="/portal/hosts/$hostId"
      params={{ hostId: host.id }}
      className="group block"
    >
      <Card className="relative overflow-hidden rounded-xl border-border/80 shadow-sm transition-all duration-200 hover:-translate-y-1 hover:shadow-md">
        <span
          className={`absolute inset-y-0 left-0 w-1 ${cfg.dot}`}
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
              SSH 密码由用户级 SSH 凭据管理，与登录密码不同。
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="rounded-lg border border-border/60 bg-card p-4 text-sm leading-relaxed text-muted-foreground shadow-sm">
              请进入具体主机详情页查看连接命令、VNC 入口和当前主机的接入方式。手动 SSH 时使用管理员分发的 SSH 密码。
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
