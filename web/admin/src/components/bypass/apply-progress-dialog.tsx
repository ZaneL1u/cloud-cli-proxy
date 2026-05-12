import { useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import { Loader2, CheckCircle2, XCircle, Circle } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { useApplyBypass } from "@/hooks/use-bypass-snapshots";
import { useTaskPolling } from "@/hooks/use-tasks";
import { useSSE } from "@/hooks/use-sse";
import { getToken } from "@/lib/auth";
import { buildSSEUrl } from "@/lib/sse-manager";
import {
  parseBypassError,
  bypassErrorMessage,
} from "@/lib/i18n/bypass-error-codes";

/** 5 个固定阶段名（46-UI-SPEC 锁定） */
const STAGES = [
  { key: "snapshot", label: "生成快照" },
  { key: "dispatch", label: "下发到 agent" },
  { key: "reload", label: "Reload 配置" },
  { key: "health", label: "健康检查" },
  { key: "done", label: "完成" },
] as const;

type StageStatus = "pending" | "active" | "done" | "failed";

interface ApplyProgressDialogProps {
  hostId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** PreviewSheet 计算出来的 risky_count，仅用于打点；不参与状态机 */
  riskyCount?: number;
}

/**
 * 应用白名单配置 Dialog：
 *
 * 5 阶段步骤条 + task.progress_percent 映射：
 * - 无 taskId（apply mutation 进行中）：snapshot=active，其它 pending
 * - apply onError：snapshot=failed，其它 pending
 * - 有 taskId 且 running：根据 task.progress_percent 推进 0%/25%/50%/75%/100%
 *   （Phase 46 占位 dispatch 直接返回 nil，task 会瞬间 succeeded，5 个阶段同时 done）
 * - task.status="succeeded"：全部 done，500ms 后自动关闭 + toast.success
 * - task.status="failed"|"canceled"：当前阶段 failed，Dialog 保持开启 + 关闭按钮
 *
 * Phase 47 接管 dispatch 后 task.progress_percent 真实更新，UI 自然展示阶段切换。
 */
export function ApplyProgressDialog({
  hostId,
  open,
  onOpenChange,
}: ApplyProgressDialogProps) {
  const applyMutation = useApplyBypass(hostId);
  const [taskId, setTaskId] = useState<string | null>(null);
  const [errorCode, setErrorCode] = useState<string | null>(null);
  const [autoCloseScheduled, setAutoCloseScheduled] = useState(false);

  const { data: task } = useTaskPolling(taskId);

  // SSE 仅用于更早唤醒 UI（task polling 已经每 2s 拉一次）。
  // 未打开时传空 url，避免组件嵌入到父容器时常驻 EventSource 订阅
  // （也让 jsdom 测试环境无需 mock window.EventSource）。
  // CR-07：必须带 token 走 ?token=... 鉴权；后端 /v1/admin/sse 现已套 adminGuard。
  useSSE(
    open ? buildSSEUrl("/v1/admin/sse", "tasks", getToken()) : "",
    () => {
      // 无需操作，react-query invalidate 已经在 useTasks 里处理
    },
  );

  // Dialog 打开 → 自动调 apply mutation；关闭 → 清空所有状态
  useEffect(() => {
    if (
      open &&
      !applyMutation.data &&
      !applyMutation.isPending &&
      !applyMutation.isError &&
      !errorCode
    ) {
      applyMutation.mutate(undefined, {
        onSuccess: (resp) => {
          setTaskId(resp.task_id);
        },
        onError: (err) => {
          const { code } = parseBypassError(err);
          setErrorCode(code ?? "TASK_FAILED");
        },
      });
    }
    if (!open) {
      applyMutation.reset();
      setTaskId(null);
      setErrorCode(null);
      setAutoCloseScheduled(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const taskStatus = task?.status;
  const isDone = taskStatus === "succeeded";
  const isFailed =
    taskStatus === "failed" || taskStatus === "canceled" || !!errorCode;
  const isRunning =
    taskId !== null &&
    taskStatus !== "succeeded" &&
    taskStatus !== "failed" &&
    taskStatus !== "canceled";

  // 5 阶段状态机
  const stageStatuses: StageStatus[] = useMemo(() => {
    if (errorCode) {
      return ["failed", "pending", "pending", "pending", "pending"];
    }
    if (!taskId) {
      return ["active", "pending", "pending", "pending", "pending"];
    }
    if (isDone) {
      return ["done", "done", "done", "done", "done"];
    }
    if (isFailed) {
      // 已有 taskId 但 task 失败：把 dispatch（第 2 阶段）标 failed
      return ["done", "failed", "pending", "pending", "pending"];
    }
    if (isRunning) {
      const pct = task?.progress_percent ?? 0;
      if (pct < 25)
        return ["done", "active", "pending", "pending", "pending"];
      if (pct < 50) return ["done", "done", "active", "pending", "pending"];
      if (pct < 75) return ["done", "done", "done", "active", "pending"];
      return ["done", "done", "done", "done", "active"];
    }
    return ["active", "pending", "pending", "pending", "pending"];
  }, [errorCode, taskId, isDone, isFailed, isRunning, task?.progress_percent]);

  // 成功 → 500ms 后自动关闭 + toast。
  // 注意：cleanup 不能 clearTimeout，否则 setAutoCloseScheduled 触发的二次 effect
  // 会取消 timer。改成：useRef 持有 timer，仅在 unmount 时清理。
  const autoCloseTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    if (isDone && !autoCloseScheduled) {
      setAutoCloseScheduled(true);
      autoCloseTimerRef.current = setTimeout(() => {
        onOpenChange(false);
        toast.success(
          "已应用 · 白名单变更不影响现有 TCP 连接，新连接才用新规则",
        );
      }, 500);
    }
  }, [isDone, autoCloseScheduled, onOpenChange]);

  // 仅在 unmount 时清掉 pending timer，避免内存泄漏
  useEffect(() => {
    return () => {
      if (autoCloseTimerRef.current) {
        clearTimeout(autoCloseTimerRef.current);
      }
    };
  }, []);

  const failureMessage = errorCode
    ? bypassErrorMessage(errorCode)
    : task?.last_error_summary || "应用过程出现错误";
  const failureCodeLabel = errorCode || task?.error_code || "TASK_FAILED";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md" data-testid="apply-progress-dialog">
        <DialogHeader>
          <DialogTitle>应用白名单配置</DialogTitle>
        </DialogHeader>

        <div className="space-y-3 py-2">
          {STAGES.map((stage, i) => {
            const status = stageStatuses[i];
            const Icon =
              status === "active"
                ? Loader2
                : status === "done"
                  ? CheckCircle2
                  : status === "failed"
                    ? XCircle
                    : Circle;
            const iconCls =
              status === "active"
                ? "text-primary animate-spin"
                : status === "done"
                  ? "text-success"
                  : status === "failed"
                    ? "text-destructive"
                    : "text-muted-foreground";
            const textCls =
              status === "active"
                ? "text-primary font-semibold"
                : status === "done"
                  ? "text-muted-foreground"
                  : status === "failed"
                    ? "text-destructive font-semibold"
                    : "text-muted-foreground";
            return (
              <div
                key={stage.key}
                data-testid={`apply-stage-${stage.key}`}
                data-status={status}
                className="flex items-center gap-3"
              >
                <Icon className={`h-5 w-5 ${iconCls}`} />
                <span className={`text-sm ${textCls}`}>{stage.label}</span>
              </div>
            );
          })}
        </div>

        {isFailed && (
          <div
            data-testid="apply-failure"
            className="rounded-md border border-destructive/30 bg-destructive/5 p-3"
          >
            <p className="text-sm font-semibold text-destructive">应用失败</p>
            <p className="mt-1 text-xs text-destructive/80">
              {failureMessage} · 错误码：
              <span className="font-mono">{failureCodeLabel}</span>
            </p>
          </div>
        )}

        {isFailed && (
          <DialogFooter>
            <Button onClick={() => onOpenChange(false)}>关闭</Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}
