import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { removeSession, saveSession, switchSession } from "@/lib/auth";
import { useAuthSessions } from "@/hooks/use-auth-sessions";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const loginSchema = z.object({
  short_id: z.string().min(1, "用户 ID 不能为空"),
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
        short_id: string;
        token: string;
        role: string;
        expires_in: number;
      }>;
    },
    onSuccess: (data) => {
      saveSession(data.short_id, data.token);
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
    <div className="flex min-h-screen flex-col items-center justify-center bg-muted/40 px-4">
      <Card className="w-full max-w-sm">
        <CardHeader className="text-center">
          <CardTitle className="text-xl">Cloud CLI Proxy</CardTitle>
          <CardDescription>登录</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="short_id">用户 ID</Label>
              <Input
                id="short_id"
                placeholder="输入 Short ID"
                autoComplete="username"
                {...register("short_id")}
              />
              {errors.short_id && (
                <p className="text-xs text-destructive">
                  {errors.short_id.message}
                </p>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="password">密码</Label>
              <Input
                id="password"
                type="password"
                autoComplete="current-password"
                {...register("password")}
              />
              {errors.password && (
                <p className="text-xs text-destructive">
                  {errors.password.message}
                </p>
              )}
            </div>

            {error && (
              <p className="text-sm text-destructive text-center">{error}</p>
            )}

            <Button
              type="submit"
              className="w-full"
              disabled={loginMutation.isPending}
            >
              {loginMutation.isPending ? "登录中…" : "登录"}
            </Button>
          </form>

          {sessions.length > 0 && (
            <div className="mt-6 space-y-3 border-t pt-4">
              <p className="text-sm font-medium">已保存会话</p>
              <div className="space-y-2">
                {sessions.map((session) => (
                  <div
                    key={session.id}
                    className="flex items-center justify-between rounded-md border p-3"
                  >
                    <div className="min-w-0">
                      <p className="font-mono text-sm">{session.shortId}</p>
                      <p className="text-xs text-muted-foreground">
                        {session.role === "admin" ? "管理员" : "用户"}
                      </p>
                    </div>
                    <div className="flex gap-2">
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() => {
                          const next = switchSession(session.id);
                          if (next?.role === "admin") {
                            navigate({ to: "/" });
                          } else {
                            navigate({ to: "/portal" });
                          }
                        }}
                      >
                        切换
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() => removeSession(session.id)}
                      >
                        删除
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </CardContent>
      </Card>
      <p className="mt-6 text-xs text-muted-foreground">
        Powered by{" "}
        <a
          href="https://github.com/zanel1u/cloud-cli-proxy"
          target="_blank"
          rel="noopener noreferrer"
          className="font-medium text-primary hover:underline"
        >
          cloud-cli-proxy
        </a>
      </p>
    </div>
  );
}
