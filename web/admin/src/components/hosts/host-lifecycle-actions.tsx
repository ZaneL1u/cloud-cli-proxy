import { useState } from "react";
import {
  Play,
  Square,
  RefreshCw,
  Trash2,
  AlertTriangle,
  ArrowUpCircle,
  Loader2,
  CheckCircle2,
  XCircle,
  ChevronDown,
  ChevronUp,
} from "lucide-react";
import { toast } from "sonner";
import { useNavigate } from "@tanstack/react-router";
import { useHostAction, useDeleteHost } from "@/hooks/use-hosts";
import type { HostImageInfo } from "@/hooks/use-hosts";
import { useTaskPolling } from "@/hooks/use-tasks";
import { useSSE } from "@/hooks/use-sse";
import { getToken } from "@/lib/auth";
import { buildSSEUrl } from "@/lib/sse-manager";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { RebuildDialog } from "./rebuild-dialog";

interface HostLifecycleActionsProps {
  hostId: string;
  hostStatus: string;
  imageInfo?: HostImageInfo;
}

export function HostLifecycleActions({
  hostId,
  hostStatus,
  imageInfo,
}: HostLifecycleActionsProps) {
  const navigate = useNavigate();
  const actionMutation = useHostAction();
  const deleteMutation = useDeleteHost();
  const [rebuildOpen, setRebuildOpen] = useState(false);
  const [upgradeOpen, setUpgradeOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [forceDeleteOpen, setForceDeleteOpen] = useState(false);
  const [upgradeTaskId, setUpgradeTaskId] = useState<string | null>(null);
  const [progressLayers, setProgressLayers] = useState<Record<string, string>>({});
  const [showLayers, setShowLayers] = useState(false);

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

  function handleAction(action: "start" | "stop") {
    actionMutation.mutate(
      { hostId, action },
      {
        onSuccess: () => toast.success("操作已提交，请查看任务状态"),
        onError: () => toast.error("操作提交失败"),
      },
    );
  }

  function handleDelete(force: boolean) {
    deleteMutation.mutate(
      { hostId, force },
      {
        onSuccess: () => {
          toast.success("主机已删除");
          navigate({ to: "/hosts" });
        },
        onError: (err: any) => {
          const msg = err?.message || "删除失败";
          if (msg.includes("运行中")) {
            toast.error("主机正在运行中，请先停止或使用强制删除");
          } else {
            toast.error(msg);
          }
        },
      },
    );
  }

  return (
    <div className="space-y-6">
      <div className="space-y-3">
        <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          运行控制
        </p>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          {(hostStatus === "stopped" || hostStatus === "failed") && (
            <Button
              className="h-11 justify-start gap-2"
              onClick={() => handleAction("start")}
              disabled={actionMutation.isPending}
            >
              <Play className="h-4 w-4 shrink-0" />
              <span className="text-left text-sm leading-snug">启动</span>
            </Button>
          )}

          {hostStatus === "running" && (
            <Button
              variant="secondary"
              className="h-11 justify-start gap-2"
              onClick={() => handleAction("stop")}
              disabled={actionMutation.isPending}
            >
              <Square className="h-4 w-4 shrink-0" />
              <span className="text-left text-sm leading-snug">停止</span>
            </Button>
          )}

          <Button
            variant="secondary"
            className="h-11 justify-start gap-2 sm:col-span-2"
            onClick={() => setRebuildOpen(true)}
            disabled={actionMutation.isPending}
          >
            <RefreshCw className="h-4 w-4 shrink-0" />
            <span className="text-left text-sm leading-snug">重建主机</span>
          </Button>

          <Button
            variant={imageInfo?.update_available ? "default" : "secondary"}
            className={`h-11 justify-start gap-2 sm:col-span-2 ${imageInfo?.update_available ? "bg-emerald-600 hover:bg-emerald-700 text-white" : ""}`}
            onClick={() => setUpgradeOpen(true)}
            disabled={actionMutation.isPending}
          >
            <ArrowUpCircle className="h-4 w-4 shrink-0" />
            <span className="text-left text-sm leading-snug">
              {imageInfo?.update_available ? "升级镜像" : "强制升级镜像"}
            </span>
            {imageInfo?.update_available && (
              <Badge variant="secondary" className="ml-auto text-[10px] px-1.5 py-0 bg-white/20 text-white">
                {imageInfo.latest_image_id}
              </Badge>
            )}
          </Button>
        </div>
      </div>

      <div className="space-y-3 rounded-xl border border-destructive/20 bg-destructive/5 p-4">
        <p className="text-xs font-semibold uppercase tracking-wide text-destructive/90">
          危险操作
        </p>
        {hostStatus !== "running" ? (
          <Button
            variant="destructive"
            className="h-11 w-full justify-start gap-2"
            onClick={() => setDeleteOpen(true)}
            disabled={deleteMutation.isPending}
          >
            <Trash2 className="h-4 w-4 shrink-0" />
            <span className="text-left text-sm leading-snug">删除主机</span>
          </Button>
        ) : (
          <Button
            variant="destructive"
            className="h-11 w-full justify-start gap-2"
            onClick={() => setForceDeleteOpen(true)}
            disabled={deleteMutation.isPending}
          >
            <AlertTriangle className="h-4 w-4 shrink-0" />
            <span className="text-left text-sm leading-snug">强制删除</span>
          </Button>
        )}
      </div>

      <RebuildDialog
        hostId={hostId}
        open={rebuildOpen}
        onOpenChange={setRebuildOpen}
      />

      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认删除主机？</AlertDialogTitle>
            <AlertDialogDescription>
              将停止并移除 Docker 容器，删除数据库中的主机记录和出口 IP
              绑定。此操作不可撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={() => handleDelete(false)}
            >
              确认删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={forceDeleteOpen} onOpenChange={setForceDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              <span className="flex items-center gap-2 text-destructive">
                <AlertTriangle className="h-5 w-5" />
                强制删除运行中的主机？
              </span>
            </AlertDialogTitle>
            <AlertDialogDescription>
              主机当前正在运行，强制删除将立即终止容器并清除所有数据。此操作不可撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={() => handleDelete(true)}
            >
              强制删除
            </AlertDialogAction>
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
    </div>
  );
}
