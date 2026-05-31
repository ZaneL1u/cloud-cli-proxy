import { useState } from "react";
import { X, Plus } from "lucide-react";
import { toast } from "sonner";
import { useUpdateHostMounts, type HostMount } from "@/hooks/use-hosts";
import { PathAutocomplete } from "@/components/hosts/path-autocomplete";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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


export function MountManager({ hostId, hostStatus, mounts }: MountManagerProps) {
  const isRunning = hostStatus === "running";
  const updateMountsMutation = useUpdateHostMounts(hostId);

  const [localMounts, setLocalMounts] = useState<HostMount[]>(mounts);

  function updateMountRow(index: number, field: "source" | "target", value: string) {
    const next = [...localMounts];
    next[index] = { ...next[index], [field]: value };
    if (field === "source" && !next[index].target) {
      next[index].target = value;
    }
    setLocalMounts(next);
  }

  function removeMountRow(index: number) {
    setLocalMounts(localMounts.filter((_, i) => i !== index));
  }

  function addMountRow() {
    setLocalMounts([...localMounts, { source: "", target: "" }]);
  }

  function handleSave() {
    const toSave = localMounts.filter(
      (m) => m.source && m.target && m.source.startsWith("/") && m.target.startsWith("/")
    );
    updateMountsMutation.mutate(toSave, {
      onSuccess: () => {
        toast.success("挂载配置已保存");
        setLocalMounts(toSave.length > 0 ? toSave : [{ source: "", target: "" }]);
      },
      onError: () => toast.error("保存失败"),
    });
  }

  const hasChanges =
    JSON.stringify(localMounts.filter((m) => m.source && m.target)) !==
    JSON.stringify(mounts);

  return (
    <div className="space-y-6">
      {!isRunning && (
        <div className="space-y-3">
          <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            挂载配置
          </p>
          <p className="text-xs text-muted-foreground">
            将宿主机目录挂载到容器内部。左侧为宿主机绝对路径，右侧为容器内挂载点，挂载后容器内可直接读写宿主机对应目录。
          </p>
          {localMounts.length > 0 && (
            <div className="rounded-xl border border-dashed border-border/60 bg-muted/20 p-4 space-y-3">
              {localMounts.map((m, i) => (
                <div key={i} className="flex items-end gap-2">
                  <div className="flex-1 space-y-1">
                    <span className="text-xs text-muted-foreground">宿主机路径</span>
                    <PathAutocomplete
                      placeholder="例: /data/shared"
                      value={m.source}
                      onChange={(v) => updateMountRow(i, "source", v)}
                      showBrowseButton
                      hostId={hostId}
                    />
                  </div>
                  <span className="pb-2 text-muted-foreground">-&gt;</span>
                  <div className="flex-1 space-y-1">
                    <span className="text-xs text-muted-foreground">容器路径</span>
                    <Input
                      placeholder="例: /data/shared"
                      value={m.target}
                      onChange={(e) => updateMountRow(i, "target", e.target.value)}
                    />
                  </div>
                  {isRunning ? (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span>
                          <Button variant="ghost" size="sm" className="h-9 w-9 p-0 shrink-0" disabled>
                            <X className="h-4 w-4" />
                          </Button>
                        </span>
                      </TooltipTrigger>
                      <TooltipContent>
                        运行中主机不允许修改挂载
                      </TooltipContent>
                    </Tooltip>
                  ) : (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="h-9 w-9 p-0 shrink-0"
                      onClick={() => removeMountRow(i)}
                    >
                      <X className="h-4 w-4" />
                    </Button>
                  )}
                </div>
              ))}
            </div>
          )}
          {!isRunning && (
            <Button
              type="button"
              variant="outline"
              className="w-full"
              onClick={addMountRow}
            >
              <Plus className="h-4 w-4 mr-2" />
              增加映射
            </Button>
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
      )}

      {isRunning && (
        <div className="space-y-3">
          <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            已配置挂载
          </p>
          {mounts.length === 0 ? (
            <p className="py-3 text-sm text-muted-foreground">尚未配置挂载路径，停止主机后可添加。</p>
          ) : (
            <ul className="space-y-3">
              {mounts.map((m, i) => (
                <li
                  key={i}
                  className="flex items-center justify-between gap-3 rounded-xl border border-border/80 bg-card px-4 py-3.5 shadow-sm"
                >
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-mono text-sm leading-tight" title={m.source}>
                      {m.source}
                    </p>
                    {m.source !== m.target && (
                      <p className="mt-1 truncate font-mono text-xs text-muted-foreground" title={m.target}>
                        → {m.target}
                      </p>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
