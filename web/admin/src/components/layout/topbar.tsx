import { useRouterState } from "@tanstack/react-router";
import { ChevronDown, CircleHelp, LogOut, Users } from "lucide-react";
import {
  clearAllSessions,
  logout,
  redirectToSessionHome,
} from "@/lib/auth";
import { useAuthSessions } from "@/hooks/use-auth-sessions";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

const pageTitles: Record<string, string> = {
  "/": "仪表板",
  "/users": "用户管理",
  "/egress-ips": "出口 IP",
  "/hosts": "主机管理",
  "/tasks": "任务列表",
  "/portal": "我的面板",
};

interface TopbarProps {
  onHelpClick?: () => void;
}

export function Topbar({ onHelpClick }: TopbarProps = {}) {
  const routerState = useRouterState();
  const pathname = routerState.location.pathname;
  const title =
    pageTitles[pathname] ??
    (pathname.startsWith("/portal/hosts/") ? "主机详情" : "管理后台");

  const { currentSession, sessions } = useAuthSessions();
  const roleLabel = currentSession?.role === "admin" ? "管理员" : "用户";

  return (
    <header className="flex h-16 items-center justify-between border-b bg-background px-6">
      <h2 className="text-lg font-semibold">{title}</h2>
      <div className="flex items-center gap-3">
        {onHelpClick && (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            onClick={onHelpClick}
            title="使用引导"
          >
            <CircleHelp className="h-5 w-5 text-muted-foreground" />
          </Button>
        )}
        <span className="text-sm text-muted-foreground">{roleLabel}</span>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button type="button" variant="outline" className="gap-2">
              <Users className="h-4 w-4" />
              <span className="max-w-32 truncate font-mono">
                {currentSession?.shortId ?? "未登录"}
              </span>
              <ChevronDown className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-72">
            <DropdownMenuLabel>已保存会话</DropdownMenuLabel>
            {sessions.map((session) => (
              <DropdownMenuItem
                key={session.id}
                onClick={() => redirectToSessionHome(session.id)}
                className="flex items-center justify-between"
              >
                <div className="min-w-0">
                  <div className="truncate font-mono text-sm">
                    {session.shortId}
                  </div>
                  <div className="text-xs text-muted-foreground">
                    {session.role === "admin" ? "管理员" : "用户"}
                  </div>
                </div>
                {session.id === currentSession?.id ? (
                  <span className="text-xs text-muted-foreground">当前</span>
                ) : null}
              </DropdownMenuItem>
            ))}
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={logout}>
              <LogOut className="h-4 w-4" />
              退出当前账号
            </DropdownMenuItem>
            <DropdownMenuItem
              variant="destructive"
              onClick={() => {
                clearAllSessions();
                window.location.href = "/login";
              }}
            >
              <LogOut className="h-4 w-4" />
              清空全部会话
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}
