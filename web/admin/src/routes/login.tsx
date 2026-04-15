import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { removeSession, saveSession, switchSession } from "@/lib/auth";
import { useAuthSessions } from "@/hooks/use-auth-sessions";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Cloud, ArrowRight, X } from "lucide-react";

const loginSchema = z.object({
  username: z.string().min(1, "用户名不能为空"),
  password: z.string().min(1, "密码不能为空"),
});

type LoginForm = z.infer<typeof loginSchema>;

export const Route = createFileRoute("/login")({
  component: LoginPage,
});

function LoginPage() {
  const navigate = useNavigate();
  const [error, setError] = useState<string | null>(null);
  const { sessions } = useAuthSessions();

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<LoginForm>({
    resolver: zodResolver(loginSchema),
  });

  const loginMutation = useMutation({
    mutationFn: async (data: LoginForm) => {
      const res = await fetch("/v1/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });

      if (!res.ok) {
        const body = await res.json().catch(() => ({ error: "登录失败" }));
        throw new Error(body.error || "登录失败");
      }

      return res.json() as Promise<{
        username: string;
        short_id: string;
        token: string;
        role: string;
        expires_in: number;
      }>;
    },
    onSuccess: (data) => {
      saveSession(data.short_id, data.token, data.username);
      if (data.role === "admin") {
        navigate({ to: "/" });
      } else {
        navigate({ to: "/portal" });
      }
    },
    onError: (err: Error) => {
      setError(err.message);
    },
  });

  const onSubmit = (data: LoginForm) => {
    setError(null);
    loginMutation.mutate(data);
  };

  return (
    <div className="flex min-h-screen">
      {/* Left: branding */}
      <div className="hidden lg:flex lg:w-1/2 flex-col justify-between bg-sidebar p-12 text-white relative overflow-hidden">
        <div className="absolute inset-0 bg-linear-to-br from-primary/20 via-transparent to-primary/5" />
        <div className="absolute -bottom-32 -right-32 h-96 w-96 rounded-full bg-primary/10 blur-3xl" />
        <div className="absolute -top-20 -left-20 h-64 w-64 rounded-full bg-primary/8 blur-2xl" />

        <div className="relative flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-primary">
            <Cloud className="h-5 w-5 text-primary-foreground" />
          </div>
          <span className="text-lg font-semibold">Cloud CLI Proxy</span>
        </div>

        <div className="relative space-y-4">
          <h1 className="text-4xl font-bold leading-tight">
            一条命令
            <br />
            一台云主机
          </h1>
          <p className="text-lg text-white/60 max-w-md leading-relaxed">
            预装 Claude Code 的隔离云主机环境，全流量走指定出口 IP，零泄漏。
          </p>
          <div className="flex gap-6 pt-4 text-sm text-white/40">
            <div className="flex items-center gap-2">
              <div className="h-1.5 w-1.5 rounded-full bg-emerald-400" />
              全隧道出网
            </div>
            <div className="flex items-center gap-2">
              <div className="h-1.5 w-1.5 rounded-full bg-emerald-400" />
              5 种代理协议
            </div>
            <div className="flex items-center gap-2">
              <div className="h-1.5 w-1.5 rounded-full bg-emerald-400" />
              多架构支持
            </div>
          </div>
        </div>

        <p className="relative text-xs text-white/30">
          © {new Date().getFullYear()} Cloud CLI Proxy · MIT License
        </p>
      </div>

      {/* Right: login form */}
      <div className="flex flex-1 flex-col items-center justify-center px-6 py-12 bg-background">
        <div className="w-full max-w-sm space-y-8">
          <div className="lg:hidden flex items-center justify-center gap-2.5 mb-2">
            <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-primary">
              <Cloud className="h-4.5 w-4.5 text-primary-foreground" />
            </div>
            <span className="text-lg font-semibold">Cloud CLI Proxy</span>
          </div>

          <div className="space-y-2 text-center lg:text-left">
            <h2 className="text-2xl font-bold tracking-tight">登录</h2>
            <p className="text-sm text-muted-foreground">
              使用用户名与登录密码进入系统
            </p>
          </div>

          <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="username">用户名</Label>
              <Input
                id="username"
                placeholder="输入用户名"
                autoComplete="username"
                className="h-11"
                {...register("username")}
              />
              {errors.username && (
                <p className="text-xs text-destructive">
                  {errors.username.message}
                </p>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="password">密码</Label>
              <Input
                id="password"
                type="password"
                placeholder="输入密码"
                autoComplete="current-password"
                className="h-11"
                {...register("password")}
              />
              {errors.password && (
                <p className="text-xs text-destructive">
                  {errors.password.message}
                </p>
              )}
            </div>

            {error && (
              <div className="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">
                {error}
              </div>
            )}

            <Button
              type="submit"
              className="w-full h-11 gap-2"
              disabled={loginMutation.isPending}
            >
              {loginMutation.isPending ? "登录中…" : "登录"}
              {!loginMutation.isPending && <ArrowRight className="h-4 w-4" />}
            </Button>
          </form>

          {sessions.length > 0 && (
            <div className="space-y-3 border-t pt-6">
              <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                已保存会话
              </p>
              <div className="space-y-2">
                {sessions.map((session) => (
                  <div
                    key={session.id}
                    className="flex items-center justify-between rounded-xl border p-3 transition-colors hover:bg-accent/50"
                  >
                    <button
                      type="button"
                      className="min-w-0 text-left"
                      onClick={() => {
                        const next = switchSession(session.id);
                        if (next?.role === "admin") {
                          navigate({ to: "/" });
                        } else {
                          navigate({ to: "/portal" });
                        }
                      }}
                    >
                      <p className="text-sm font-medium">
                        {session.username ?? session.shortId}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        {session.role === "admin" ? "管理员" : "用户"}
                      </p>
                    </button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8 text-muted-foreground hover:text-destructive"
                      onClick={() => removeSession(session.id)}
                    >
                      <X className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
