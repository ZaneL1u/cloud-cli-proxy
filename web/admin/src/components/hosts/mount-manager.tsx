import { useState } from "react";
import { X, Plus, AlertTriangle } from "lucide-react";
import { toast } from "sonner";
import { useUpdateHostMounts, type HostMount } from "@/hooks/use-hosts";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

interface MountManagerProps {
  hostId: string;
  hostStatus: string;
  mounts: HostMount[];
}

const RESERVED_PATHS = ["/workspace", "/home/claude", "/var/lib/claude-persist"];

export function MountManager({ hostId, hostStatus, mounts }: MountManagerProps) {
  const isRunning = hostStatus === "running";
  const updateMountsMutation = useUpdateHostMounts(hostId);

  const [localMounts, setLocalMounts] = useState<HostMount[]>(mounts);
  const [newSource, setNewSource] = useState("");
  const [newTarget, setNewTarget] = useState("");
  const [prevSource, setPrevSource] = useState("");

  function handleAdd() {
    if (!newSource.startsWith("/") || !newTarget.startsWith("/")) return;
    setLocalMounts([...localMounts, { source: newSource, target: newTarget }]);
    setNewSource("");
    setNewTarget("");
    setPrevSource("");
  }

  function handleRemove(index: number) {
    setLocalMounts(localMounts.filter((_, i) => i !== index));
  }

  function handleSave() {
    updateMountsMutation.mutate(localMounts, {
      onSuccess: () => toast.success("挂载配置已保存"),
      onError: () => toast.error("保存失败"),
    });
  }

  const hasChanges =
    JSON.stringify(localMounts) !== JSON.stringify(mounts);

  return (
    <div className="space-y-6">
      <div className="space-y-3">
        <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          已配置挂载
        </p>
        {localMounts.length === 0 ? (
          <div className="rounded-xl border border-dashed border-border/80 bg-muted/20 px-4 py-8 text-center">
            <p className="text-sm text-muted-foreground">暂无挂载配置</p>
            <p className="mt-1 text-xs text-muted-foreground">
              在下方填写宿主机路径，添加后保存即可生效。
            </p>
          </div>
        ) : (
          <ul className="space-y-3">
            {localMounts.map((m, i) => (
              <li
                key={i}
                className="flex items-center justify-between gap-3 rounded-xl border border-border/80 bg-card px-4 py-3.5 shadow-sm"
              >
                <div className="min-w-0 flex-1">
                  <p className="truncate font-mono text-sm leading-tight">
                    {m.source}
                  </p>
                  {m.source !== m.target && (
                    <p className="mt-1 font-mono text-xs text-muted-foreground">
                      → {m.target}
                    </p>
                  )}
                </div>
                {isRunning ? (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span>
                        <Button variant="ghost" size="icon" className="shrink-0" disabled>
                          <X className="h-4 w-4" />
                        </Button>
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>
                      运行中主机不允许修改挂载，请先停止主机
                    </TooltipContent>
                  </Tooltip>
                ) : (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="shrink-0"
                    onClick={() => handleRemove(i)}
                  >
                    <X className="h-4 w-4" />
                  </Button>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>

      {!isRunning && (
        <div className="rounded-xl border border-border/60 bg-muted/20 p-4">
          <Label className="text-xs font-semibold text-foreground">
            添加挂载
          </Label>
          <div className="mt-3 space-y-3">
            <div className="grid gap-3 sm:grid-cols-2">
              <Input
                placeholder="宿主机路径 (例: /data/shared)"
                value={newSource}
                onChange={(e) => {
                  setNewSource(e.target.value);
                  if (!newTarget || newTarget === prevSource) {
                    setNewTarget(e.target.value);
                  }
                  setPrevSource(e.target.value);
                }}
              />
              <Input
                placeholder="容器路径 (默认同宿主机路径)"
                value={newTarget}
                onChange={(e) => setNewTarget(e.target.value)}
              />
            </div>
            {newSource && !newSource.startsWith("/") && (
              <p className="text-xs text-destructive">宿主机路径必须以 / 开头</p>
            )}
            {newTarget && !newTarget.startsWith("/") && (
              <p className="text-xs text-destructive">容器路径必须以 / 开头</p>
            )}
            {RESERVED_PATHS.includes(newTarget) && (
              <p className="flex items-center gap-1.5 text-xs text-yellow-600">
                <AlertTriangle className="h-3.5 w-3.5" />
                该路径为系统保留路径，可能影响容器正常运行
              </p>
            )}
            <Button
              type="button"
              variant="outline"
              className="gap-2"
              disabled={!newSource.startsWith("/") || !newTarget.startsWith("/")}
              onClick={handleAdd}
            >
              <Plus className="h-4 w-4" />
              添加
            </Button>
          </div>
        </div>
      )}

      {!isRunning && hasChanges && (
        <Button
          type="button"
          onClick={handleSave}
          disabled={updateMountsMutation.isPending}
        >
          {updateMountsMutation.isPending ? "保存中..." : "保存挂载配置"}
        </Button>
      )}
    </div>
  );
}
