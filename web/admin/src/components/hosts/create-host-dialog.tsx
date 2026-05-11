import { useState, useEffect, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Loader2, CheckCircle2, XCircle, AlertCircle, X } from "lucide-react";
import { useUsers } from "@/hooks/use-users";
import { useCreateHost } from "@/hooks/use-hosts";
import { useEgressIPs } from "@/hooks/use-egress-ips";
import { useTaskPolling } from "@/hooks/use-tasks";
import { PathAutocomplete } from "@/components/hosts/path-autocomplete";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

interface CreateHostDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

const TIMEZONE_OPTIONS = [
  { value: "America/Los_Angeles", label: "美西 / 洛杉矶", offset: "UTC-8" },
  { value: "America/New_York", label: "美东 / 纽约", offset: "UTC-5" },
  { value: "America/Chicago", label: "美中 / 芝加哥", offset: "UTC-6" },
  { value: "America/Denver", label: "山区 / 丹佛", offset: "UTC-7" },
  { value: "Europe/London", label: "伦敦", offset: "UTC+0" },
  { value: "Europe/Paris", label: "巴黎", offset: "UTC+1" },
  { value: "Europe/Berlin", label: "柏林", offset: "UTC+1" },
  { value: "Asia/Tokyo", label: "东京", offset: "UTC+9" },
  { value: "Asia/Shanghai", label: "上海", offset: "UTC+8" },
  { value: "Asia/Singapore", label: "新加坡", offset: "UTC+8" },
  { value: "Asia/Seoul", label: "首尔", offset: "UTC+9" },
  { value: "Australia/Sydney", label: "悉尼", offset: "UTC+10" },
  { value: "Pacific/Honolulu", label: "夏威夷", offset: "UTC-10" },
];

const statusDisplay: Record<
  string,
  { icon: React.ReactNode; label: string; color: string }
> = {
  pending: {
    icon: <Loader2 className="h-5 w-5 animate-spin" />,
    label: "排队中…",
    color: "text-muted-foreground",
  },
  running: {
    icon: <Loader2 className="h-5 w-5 animate-spin text-primary" />,
    label: "创建中…",
    color: "text-primary",
  },
  succeeded: {
    icon: <CheckCircle2 className="h-5 w-5 text-green-600" />,
    label: "创建成功",
    color: "text-green-600",
  },
  failed: {
    icon: <XCircle className="h-5 w-5 text-destructive" />,
    label: "创建失败",
    color: "text-destructive",
  },
  canceled: {
    icon: <AlertCircle className="h-5 w-5 text-muted-foreground" />,
    label: "已取消",
    color: "text-muted-foreground",
  },
};

export function CreateHostDialog({
  open,
  onOpenChange,
}: CreateHostDialogProps) {
  const [userId, setUserId] = useState("");
  const [egressIpId, setEgressIpId] = useState("");
  const [timezone, setTimezone] = useState("America/Los_Angeles");
  const [hostMounts, setHostMounts] = useState<Array<{ source: string; target: string }>>([
    { source: "", target: "" },
  ]);
  const [taskId, setTaskId] = useState<string | null>(null);
  const { data: usersData, isLoading: loadingUsers } = useUsers();
  const { data: egressData, isLoading: loadingEgress } = useEgressIPs();
  const createMutation = useCreateHost();
  const { data: task } = useTaskPolling(taskId);

  const isTracking = !!taskId;
  const taskStatus = task?.status ?? "pending";

  const qc = useQueryClient();
  const prevTaskStatus = useRef<string | null>(null);

  useEffect(() => {
    const prev = prevTaskStatus.current;
    prevTaskStatus.current = taskStatus;
    if (prev && prev !== taskStatus && (taskStatus === "succeeded" || taskStatus === "failed")) {
      qc.invalidateQueries({ queryKey: ["hosts"] });
      qc.invalidateQueries({ queryKey: ["dashboard-stats"] });
    }
  }, [taskStatus, qc]);

  const users = usersData?.users ?? [];
  const activeUsers = users.filter((u) => u.status === "active");
  const egressIPs = (egressData?.egress_ips ?? []).filter(
    (ip: any) => ip.status === "available",
  );
  const isDone =
    taskStatus === "succeeded" ||
    taskStatus === "failed" ||
    taskStatus === "canceled";
  const display = statusDisplay[taskStatus] ?? statusDisplay.pending;

  function handleSubmit() {
    if (!userId) {
      toast.error("请选择用户");
      return;
    }
    if (!egressIpId) {
      toast.error("请选择出口 IP");
      return;
    }
    const mounts = hostMounts
      .filter((m) => m.source && m.target && m.source.startsWith("/") && m.target.startsWith("/"));
    createMutation.mutate(
      { user_id: userId, egress_ip_id: egressIpId, timezone, host_mounts: mounts.length > 0 ? mounts : undefined },
      {
        onSuccess: (data) => {
          setTaskId(data.task_id);
        },
        onError: () => toast.error("提交失败"),
      },
    );
  }

  function handleClose() {
    setUserId("");
    setEgressIpId("");
    setTimezone("America/Los_Angeles");
    setHostMounts([{ source: "", target: "" }]);
    setTaskId(null);
    onOpenChange(false);
  }

  return (
    <Dialog
      open={open}
      onOpenChange={isTracking && !isDone ? undefined : handleClose}
    >
      <DialogContent className="sm:max-w-[560px]">
        <DialogHeader>
          <DialogTitle>新建主机</DialogTitle>
        </DialogHeader>

        {!isTracking ? (
          <>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label>所属用户 <span className="text-destructive">*</span></Label>
                {loadingUsers ? (
                  <div className="h-9 animate-pulse rounded-md bg-muted" />
                ) : (
                  <Select value={userId} onValueChange={setUserId}>
                    <SelectTrigger>
                      <SelectValue placeholder="选择用户" />
                    </SelectTrigger>
                    <SelectContent>
                      {activeUsers.map((user) => (
                        <SelectItem key={user.id} value={user.id}>
                          {user.username}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
                {activeUsers.length === 0 && !loadingUsers && (
                  <p className="text-sm text-muted-foreground">
                    没有可用的活跃用户，请先创建用户
                  </p>
                )}
              </div>

              <div className="space-y-2">
                <Label>出口 IP <span className="text-destructive">*</span></Label>
                {loadingEgress ? (
                  <div className="h-9 animate-pulse rounded-md bg-muted" />
                ) : (
                  <Select value={egressIpId} onValueChange={setEgressIpId}>
                    <SelectTrigger>
                      <SelectValue placeholder="选择出口 IP" />
                    </SelectTrigger>
                    <SelectContent>
                      {egressIPs.map((ip: any) => (
                        <SelectItem key={ip.id} value={ip.id}>
                          <span className="font-mono">{ip.ip_address}</span>
                          {ip.ip_address !== "0.0.0.0" && (
                            <span className="ml-2 text-muted-foreground">
                              {ip.label}
                            </span>
                          )}
                          {ip.ip_address === "0.0.0.0" && (
                            <span className="ml-2 text-muted-foreground">
                              {ip.label}（待检测）
                            </span>
                          )}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
                {egressIPs.length === 0 && !loadingEgress && (
                  <p className="text-sm text-muted-foreground">
                    没有可用的出口 IP，请先添加
                  </p>
                )}
              </div>

              <div className="space-y-2">
                <Label>时区</Label>
                <Select value={timezone} onValueChange={setTimezone}>
                  <SelectTrigger>
                    <SelectValue placeholder="选择时区" />
                  </SelectTrigger>
                  <SelectContent>
                    {TIMEZONE_OPTIONS.map((tz) => (
                      <SelectItem key={tz.value} value={tz.value}>
                        {tz.label}
                        <span className="ml-1.5 text-muted-foreground">
                          ({tz.offset})
                        </span>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-2">
                <Label>挂载路径（可选）</Label>
                <p className="text-xs text-muted-foreground">
                  将宿主机目录挂载到容器内部。左侧为宿主机绝对路径，右侧为容器内挂载点，挂载后容器内可直接读写宿主机对应目录。
                </p>
                <div className="rounded-xl border border-dashed border-border/60 bg-muted/20 p-4 space-y-3">
                  {hostMounts.map((m, i) => (
                    <div key={i} className="flex items-end gap-2">
                      <div className="flex-1 space-y-1">
                        <span className="text-xs text-muted-foreground">宿主机路径</span>
                        <PathAutocomplete
                          placeholder="例: /data/shared"
                          value={m.source}
                          onChange={(v) => {
                            const next = [...hostMounts];
                            next[i] = { ...next[i], source: v };
                            if (!next[i].target) next[i].target = v;
                            setHostMounts(next);
                          }}
                        />
                      </div>
                      <span className="pb-2 text-muted-foreground">-&gt;</span>
                      <div className="flex-1 space-y-1">
                        <span className="text-xs text-muted-foreground">容器路径</span>
                        <Input
                          placeholder="例: /data/shared"
                          value={m.target}
                          onChange={(e) => {
                            const next = [...hostMounts];
                            next[i] = { ...next[i], target: e.target.value };
                            setHostMounts(next);
                          }}
                        />
                      </div>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="h-9 w-9 p-0 shrink-0"
                        onClick={() => setHostMounts(hostMounts.filter((_, j) => j !== i))}
                      >
                        <X className="h-4 w-4" />
                      </Button>
                    </div>
                  ))}
                </div>
                <Button
                  type="button"
                  variant="outline"
                  className="w-full"
                  onClick={() => setHostMounts([...hostMounts, { source: "", target: "" }])}
                >
                  增加映射
                </Button>
              </div>

            </div>

            <DialogFooter>
              <Button variant="outline" onClick={handleClose}>
                取消
              </Button>
              <Button
                onClick={handleSubmit}
                disabled={
                  !userId || !egressIpId || createMutation.isPending
                }
              >
                {createMutation.isPending ? "提交中..." : "创建"}
              </Button>
            </DialogFooter>
          </>
        ) : (
          <div className="space-y-4 py-6">
            <div className="flex items-center gap-3">
              {display.icon}
              <div className="flex-1">
                <p className={`font-medium ${display.color}`}>
                  {display.label}
                </p>
                <p className="text-xs text-muted-foreground">
                  任务 {taskId?.slice(0, 8)}…
                </p>
              </div>
            </div>

            {taskStatus === "running" && (task?.progress_percent ?? 0) > 0 && (
              <div className="space-y-1.5">
                <div className="flex items-center justify-between text-xs">
                  <span className="text-muted-foreground">
                    {task?.progress_message || "处理中…"}
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

            {taskStatus === "failed" && task?.last_error_summary && (
              <div className="rounded-md border border-destructive/30 bg-destructive/5 p-3">
                <p className="text-sm font-medium text-destructive">
                  错误详情
                </p>
                <p className="mt-1 break-all text-xs text-destructive/80">
                  {task.last_error_summary}
                </p>
              </div>
            )}

            {taskStatus === "failed" &&
              task?.error_message &&
              task.error_message !== task.last_error_summary && (
                <details className="text-xs text-muted-foreground">
                  <summary className="cursor-pointer hover:text-foreground">
                    完整错误信息
                  </summary>
                  <pre className="mt-2 max-h-40 overflow-auto whitespace-pre-wrap break-all rounded bg-muted p-2 text-xs">
                    {task.error_message}
                  </pre>
                </details>
              )}

            <DialogFooter>
              {isDone && (
                <Button onClick={handleClose}>
                  {taskStatus === "succeeded" ? "完成" : "关闭"}
                </Button>
              )}
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
