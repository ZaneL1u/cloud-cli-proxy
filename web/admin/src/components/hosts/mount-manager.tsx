import { useState } from "react";
import { X, Plus, AlertTriangle } from "lucide-react";
import { toast } from "sonner";
import { useUpdateHostMounts, type HostMount } from "@/hooks/use-hosts";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

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
  const [newReadOnly, setNewReadOnly] = useState(false);

  function handleAdd() {
    if (!newSource.startsWith("/") || !newTarget.startsWith("/")) return;
    setLocalMounts([...localMounts, { source: newSource, target: newTarget, read_only: newReadOnly }]);
    setNewSource("");
    setNewTarget("");
    setPrevSource("");
    setNewReadOnly(false);
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

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        {localMounts.length === 0 ? (
          <p className="py-2 text-sm text-muted-foreground">暂无挂载配置</p>
        ) : (
          <ul className="divide-y divide-border/60">
            {localMounts.map((m, i) => (
              <li key={i} className="flex items-center justify-between gap-3 py-2.5">
                <div className="min-w-0 flex-1">
                  <span className="font-mono text-sm">{m.source}</span>
                  <span className="mx-2 text-muted-foreground">-&gt;</span>
                  <span className="font-mono text-sm">{m.target}</span>
                  {m.read_only && (
                    <span className="ml-2 text-xs text-muted-foreground">(只读)</span>
                  )}
                </div>
                {!isRunning && (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7 shrink-0"
                    onClick={() => handleRemove(i)}
                  >
                    <X className="h-3.5 w-3.5" />
                  </Button>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>

      {!isRunning && (
        <div className="space-y-2">
          <Label>添加挂载</Label>
          <div className="flex items-end gap-2">
            <div className="flex-1 space-y-1">
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
            </div>
            <span className="pb-2 text-muted-foreground">-&gt;</span>
            <div className="flex-1 space-y-1">
              <Input
                placeholder="容器路径 (默认同宿主机路径)"
                value={newTarget}
                onChange={(e) => setNewTarget(e.target.value)}
              />
            </div>
            <div className="flex items-center gap-1.5 pb-1.5">
              <input
                type="checkbox"
                id="mount-readonly"
                checked={newReadOnly}
                onChange={(e) => setNewReadOnly(e.target.checked)}
                className="h-4 w-4 rounded border-border accent-primary"
              />
              <Label htmlFor="mount-readonly" className="text-xs whitespace-nowrap">只读</Label>
            </div>
            <Button
              type="button"
              variant="outline"
              className="h-9"
              disabled={!newSource.startsWith("/") || !newTarget.startsWith("/")}
              onClick={handleAdd}
            >
              <Plus className="h-4 w-4" />
            </Button>
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
        </div>
      )}

      {!isRunning && (
        <Button
          type="button"
          onClick={handleSave}
          disabled={updateMountsMutation.isPending}
        >
          {updateMountsMutation.isPending ? "保存中..." : "保存"}
        </Button>
      )}
    </div>
  );
}
