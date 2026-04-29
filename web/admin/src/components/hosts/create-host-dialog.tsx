import { useState, useEffect, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Loader2, CheckCircle2, XCircle, AlertCircle, X, Plus } from "lucide-react";
import { useUsers } from "@/hooks/use-users";
import { useCreateHost } from "@/hooks/use-hosts";
import { useEgressIPs } from "@/hooks/use-egress-ips";
import { useTaskPolling } from "@/hooks/use-tasks";
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
  const [hostMounts, setHostMounts] = useState<Array<{ source: string; target: string; read_only?: boolean }>>([]);
  const [newMountSource, setNewMountSource] = useState("");
  const [newMountTarget, setNewMountTarget] = useState("");
  const [prevMountSource, setPrevMountSource] = useState("");
  const [taskId, setTaskId] = useState<string | null>(null);
  const [hostAccess, setHostAccess] = useState<{
    shortId: string;
    entryPassword: string;
  } | null>(null);
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
    createMutation.mutate(
      { user_id: userId, egress_ip_id: egressIpId, timezone, host_mounts: hostMounts.length > 0 ? hostMounts : undefined },
      {
        onSuccess: (data: any) => {
          setTaskId(data.task_id);
          setHostAccess({
            shortId: data.short_id,
            entryPassword: data.entry_password,
          });
        },
        onError: () => toast.error("提交失败"),
      },
    );
  }

  function handleClose() {
    setUserId("");
    setEgressIpId("");
    setTimezone("America/Los_Angeles");
    setHostMounts([]);
    setNewMountSource("");
    setNewMountTarget("");
    setPrevMountSource("");
    setTaskId(null);
    setHostAccess(null);
    onOpenChange(false);
  }

  return (
    <Dialog
      open={open}
      onOpenChange={isTracking && !isDone ? undefined : handleClose}
    >
      <DialogContent className="sm:max-w-[420px]">
        <DialogHeader>
          <DialogTitle>新建主机</DialogTitle>
        </DialogHeader>

        {!isTracking ? (
          <>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label>所属用户 *</Label>
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
                <Label>出口 IP *</Label>
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
                    <SelectItem value="America/Los_Angeles">美西 / 洛杉矶</SelectItem>
                    <SelectItem value="America/New_York">美东 / 纽约</SelectItem>
                    <SelectItem value="America/Chicago">美中 / 芝加哥</SelectItem>
                    <SelectItem value="America/Denver">山区 / 丹佛</SelectItem>
                    <SelectItem value="Europe/London">伦敦</SelectItem>
                    <SelectItem value="Europe/Paris">巴黎</SelectItem>
                    <SelectItem value="Europe/Berlin">柏林</SelectItem>
                    <SelectItem value="Asia/Tokyo">东京</SelectItem>
                    <SelectItem value="Asia/Shanghai">上海</SelectItem>
                    <SelectItem value="Asia/Singapore">新加坡</SelectItem>
                    <SelectItem value="Asia/Seoul">首尔</SelectItem>
                    <SelectItem value="Australia/Sydney">悉尼</SelectItem>
                    <SelectItem value="Pacific/Honolulu">夏威夷</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-2">
                <Label>挂载路径（可选）</Label>
                {hostMounts.map((m, i) => (
                  <div key={i} className="flex items-center gap-2 text-sm">
                    <span className="truncate font-mono" title={m.source}>{m.source}</span>
                    <span className="text-muted-foreground">-&gt;</span>
                    <span className="truncate font-mono" title={m.target}>{m.target}</span>
                    {m.read_only && <span className="text-xs text-muted-foreground">(只读)</span>}
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="ml-auto h-6 w-6 p-0"
                      onClick={() => setHostMounts(hostMounts.filter((_, j) => j !== i))}
                    >
                      <X className="h-3 w-3" />
                    </Button>
                  </div>
                ))}
                <div className="flex items-end gap-2">
                  <div className="flex-1 space-y-1">
                    <Input
                      placeholder="宿主机路径 (例: /data/shared)"
                      value={newMountSource}
                      onChange={(e) => {
                        setNewMountSource(e.target.value);
                        if (!newMountTarget || newMountTarget === prevMountSource) {
                          setNewMountTarget(e.target.value);
                        }
                        setPrevMountSource(e.target.value);
                      }}
                    />
                  </div>
                  <span className="pb-2 text-muted-foreground">-&gt;</span>
                  <div className="flex-1 space-y-1">
                    <Input
                      placeholder="容器路径 (默认同宿主机路径)"
                      value={newMountTarget}
                      onChange={(e) => setNewMountTarget(e.target.value)}
                    />
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    className="h-9"
                    disabled={!newMountSource.startsWith("/") || !newMountTarget.startsWith("/")}
                    onClick={() => {
                      setHostMounts([...hostMounts, { source: newMountSource, target: newMountTarget }]);
                      setNewMountSource("");
                      setNewMountTarget("");
                      setPrevMountSource("");
                    }}
                  >
                    <Plus className="h-4 w-4" />
                  </Button>
                </div>
                {newMountSource && !newMountSource.startsWith("/") && (
                  <p className="text-xs text-destructive">宿主机路径必须以 / 开头</p>
                )}
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

            {hostAccess && (
              <div className="rounded-md border bg-muted/50 p-3 text-sm">
                <p className="font-medium">主机 SSH 凭据（仅展示一次）</p>
                <div className="mt-2 space-y-1 font-mono text-xs">
                  <p>主机短 ID：{hostAccess.shortId}</p>
                  <p>SSH 密码：{hostAccess.entryPassword}</p>
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
