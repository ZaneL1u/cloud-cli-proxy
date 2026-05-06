import { useState } from "react";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  MoreHorizontal,
  Eye,
  Plus,
  Trash2,
  Monitor,
  Play,
  Square,
  RotateCcw,
  Globe,
  Server,
  Loader2,
} from "lucide-react";
import { toast } from "sonner";
import { getToken } from "@/lib/auth";
import { useHosts, useDeleteHost, useHostAction } from "@/hooks/use-hosts";
import { useTasks } from "@/hooks/use-tasks";
import { CreateHostDialog } from "@/components/hosts/create-host-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
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
import { PageHeader } from "@/components/layout/page-header";
import { DataTableShell } from "@/components/layout/data-table-shell";
import { EmptyState } from "@/components/layout/empty-state";

export const Route = createFileRoute("/_dashboard/hosts/")({
  component: HostsPage,
});

function formatDate(dateStr: string) {
  const d = new Date(dateStr);
  return d.toLocaleDateString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

const taskKindLabels: Record<string, string> = {
  create_host: "创建",
  start_host: "启动",
  stop_host: "停止",
  rebuild_host: "重建",
};

function getHostStatus(
  host: (typeof useHosts extends (...args: any[]) => { data: infer D } | undefined ? D : never)["hosts"][number],
  latestTask?: ReturnType<typeof useTasks>["data"]["tasks"][number],
) {
  // 优先显示进行中的任务
  if (latestTask && (latestTask.status === "pending" || latestTask.status === "running")) {
    const kind = taskKindLabels[latestTask.kind] ?? latestTask.kind;
    return { type: "loading" as const, label: `${kind}中...` };
  }

  // 以 DB status 为唯一数据源
  const db = host.status;
  if (db === "failed") return { type: "badge" as const, label: "失败", variant: "destructive" as const };
  if (db === "pending") return { type: "badge" as const, label: "等待中", variant: "outline" as const };
  if (db === "running") return { type: "badge" as const, label: "运行中", variant: "default" as const };
  if (db === "stopped") return { type: "badge" as const, label: "已停止", variant: "secondary" as const };

  return { type: "badge" as const, label: db || "未知", variant: "outline" as const };
}

function HostsPage() {
  const { data, isLoading } = useHosts();
  const { data: tasksData } = useTasks();
  const hosts = data?.hosts ?? [];
  const tasks = tasksData?.tasks ?? [];
  const deleteMutation = useDeleteHost();
  const hostAction = useHostAction();
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<{
    id: string;
    username: string;
    status: string;
  } | null>(null);

  function getLatestTask(hostId: string) {
    return tasks.find((t) => t.host_id === hostId);
  }

  function handleAction(
    hostId: string,
    action: "start" | "stop" | "rebuild",
    label: string,
  ) {
    hostAction.mutate(
      { hostId, action },
      {
        onSuccess: () => toast.success(`${label}指令已发送`),
        onError: () => toast.error(`${label}失败`),
      },
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="主机管理"
        description="查看并管理所有用户云主机、容器状态与生命周期操作"
      >
        <Button onClick={() => setCreateOpen(true)}>
          <Plus className="mr-2 h-4 w-4" />
          新建主机
        </Button>
      </PageHeader>

      <CreateHostDialog open={createOpen} onOpenChange={setCreateOpen} />

      <DataTableShell>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>主机名</TableHead>
              <TableHead>所属用户</TableHead>
              <TableHead>出口 IP</TableHead>
              <TableHead>运行状态</TableHead>
              <TableHead>更新时间</TableHead>
              <TableHead className="w-[140px]">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              Array.from({ length: 3 }).map((_, i) => (
                <TableRow key={i}>
                  {Array.from({ length: 6 }).map((_, j) => (
                    <TableCell key={j}>
                      <div className="h-4 w-20 animate-pulse rounded bg-muted" />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : hosts.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="p-0">
                  <EmptyState
                    icon={Server}
                    title="暂无主机"
                    description="创建主机后，可在此查看容器状态、出口 IP 绑定与运维操作"
                    action={
                      <Button onClick={() => setCreateOpen(true)}>
                        <Plus className="mr-2 h-4 w-4" />
                        新建主机
                      </Button>
                    }
                  />
                </TableCell>
              </TableRow>
            ) : (
              hosts.map((host) => {
                const latestTask = getLatestTask(host.id);
                const status = getHostStatus(host, latestTask);
                const isRunning = host.status === "running";
                const isStopped =
                  host.status === "stopped" ||
                  host.status === "failed" ||
                  host.status === "not found";

                return (
                  <TableRow key={host.id}>
                    <TableCell>
                      <Link
                        to="/hosts/$hostId"
                        params={{ hostId: host.id }}
                        className="font-mono text-sm text-primary hover:underline"
                      >
                        {host.hostname || host.id.slice(0, 8) + "…"}
                      </Link>
                    </TableCell>
                    <TableCell>{host.username}</TableCell>
                    <TableCell>
                      {host.egress_ip_label ? (
                        <TooltipProvider>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span className="inline-flex items-center gap-1 text-sm">
                                <Globe className="h-3.5 w-3.5 text-muted-foreground" />
                                {host.egress_ip_label}
                              </span>
                            </TooltipTrigger>
                            <TooltipContent>
                              {host.egress_ip_address}
                            </TooltipContent>
                          </Tooltip>
                        </TooltipProvider>
                      ) : (
                        <span className="text-sm text-muted-foreground">
                          未绑定
                        </span>
                      )}
                    </TableCell>
                    <TableCell>
                      {status.type === "loading" && latestTask ? (
                        <TooltipProvider delayDuration={100}>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span className="inline-flex items-center gap-1.5 text-sm text-primary cursor-help">
                                <Loader2 className="h-3.5 w-3.5 animate-spin" />
                                {status.label}
                              </span>
                            </TooltipTrigger>
                            <TooltipContent side="bottom" className="max-w-xs">
                              <div className="space-y-1">
                                <p className="text-xs font-medium">
                                  任务: {taskKindLabels[latestTask.kind] ?? latestTask.kind}
                                </p>
                                <p className="text-xs text-muted-foreground">
                                  状态: {latestTask.status === "pending" ? "排队中" : "执行中"}
                                </p>
                                {latestTask.last_error_summary && (
                                  <p className="text-xs text-destructive break-all">
                                    错误: {latestTask.last_error_summary}
                                  </p>
                                )}
                                {latestTask.updated_at && (
                                  <p className="text-xs text-muted-foreground">
                                    更新: {formatDate(latestTask.updated_at)}
                                  </p>
                                )}
                              </div>
                            </TooltipContent>
                          </Tooltip>
                        </TooltipProvider>
                      ) : status.type === "loading" ? (
                        <span className="inline-flex items-center gap-1.5 text-sm text-primary">
                          <Loader2 className="h-3.5 w-3.5 animate-spin" />
                          {status.label}
                        </span>
                      ) : (
                        <Badge variant={status.variant}>{status.label}</Badge>
                      )}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {formatDate(host.updated_at)}
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-0.5">
                        {isStopped && (
                          <TooltipProvider>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <Button
                                  variant="ghost"
                                  size="icon"
                                  className="h-8 w-8"
                                  disabled={hostAction.isPending}
                                  onClick={() =>
                                    handleAction(host.id, "start", "启动")
                                  }
                                >
                                  <Play className="h-4 w-4 text-green-600" />
                                </Button>
                              </TooltipTrigger>
                              <TooltipContent>启动</TooltipContent>
                            </Tooltip>
                          </TooltipProvider>
                        )}
                        {isRunning && (
                          <>
                            <TooltipProvider>
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <Button
                                    variant="ghost"
                                    size="icon"
                                    className="h-8 w-8"
                                    disabled={hostAction.isPending}
                                    onClick={() =>
                                      handleAction(host.id, "stop", "停止")
                                    }
                                  >
                                    <Square className="h-4 w-4 text-red-500" />
                                  </Button>
                                </TooltipTrigger>
                                <TooltipContent>停止</TooltipContent>
                              </Tooltip>
                            </TooltipProvider>
                            <TooltipProvider>
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <Button
                                    variant="ghost"
                                    size="icon"
                                    className="h-8 w-8"
                                    disabled={hostAction.isPending}
                                    onClick={() =>
                                      handleAction(host.id, "rebuild", "重建")
                                    }
                                  >
                                    <RotateCcw className="h-4 w-4 text-blue-500" />
                                  </Button>
                                </TooltipTrigger>
                                <TooltipContent>重建</TooltipContent>
                              </Tooltip>
                            </TooltipProvider>
                            <TooltipProvider>
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <Button
                                    variant="ghost"
                                    size="icon"
                                    className="h-8 w-8"
                                    onClick={() => {
                                      const token = getToken();
                                      const wsPath = encodeURIComponent(
                                        `v1/admin/hosts/${host.id}/vnc/`,
                                      );
                                      window.open(
                                        `/v1/admin/hosts/${host.id}/vnc/vnc.html?autoconnect=true&resize=remote&path=${wsPath}&token=${token}`,
                                        "_blank",
                                      );
                                    }}
                                  >
                                    <Monitor className="h-4 w-4" />
                                  </Button>
                                </TooltipTrigger>
                                <TooltipContent>浏览器桌面</TooltipContent>
                              </Tooltip>
                            </TooltipProvider>
                          </>
                        )}
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8"
                            >
                              <MoreHorizontal className="h-4 w-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            <DropdownMenuItem asChild>
                              <Link
                                to="/hosts/$hostId"
                                params={{ hostId: host.id }}
                              >
                                <Eye />
                                查看详情
                              </Link>
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem
                              variant="destructive"
                              onClick={() =>
                                setDeleteTarget({
                                  id: host.id,
                                  username: host.username,
                                  status: host.status,
                                })
                              }
                            >
                              <Trash2 />
                              删除
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })
            )}
          </TableBody>
        </Table>
      </DataTableShell>

      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {deleteTarget?.status === "running"
                ? "强制删除运行中的主机？"
                : "确认删除主机？"}
            </AlertDialogTitle>
            <AlertDialogDescription>
              将移除用户 {deleteTarget?.username}{" "}
              的主机容器和数据库记录。此操作不可撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={() => {
                if (!deleteTarget) return;
                deleteMutation.mutate(
                  {
                    hostId: deleteTarget.id,
                    force: deleteTarget.status === "running",
                  },
                  {
                    onSuccess: () => {
                      toast.success("主机已删除");
                      setDeleteTarget(null);
                    },
                    onError: () => {
                      toast.error("删除失败");
                      setDeleteTarget(null);
                    },
                  },
                );
              }}
            >
              {deleteTarget?.status === "running" ? "强制删除" : "确认删除"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

