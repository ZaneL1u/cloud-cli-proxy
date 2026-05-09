import { useState } from "react";
import { X, Plus } from "lucide-react";
import { toast } from "sonner";
import { useUpdateHostPorts, type HostPort } from "@/hooks/use-hosts";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

interface PortRow {
  host_port: string;
  container_port: string;
}

interface PortManagerProps {
  hostId: string;
  hostStatus: string;
  ports: HostPort[];
}

function portRowValid(p: PortRow): boolean {
  const hp = parseInt(p.host_port, 10);
  const cp = parseInt(p.container_port, 10);
  return (
    !isNaN(hp) && hp > 0 && hp <= 65535 &&
    !isNaN(cp) && cp > 0 && cp <= 65535
  );
}

export function PortManager({ hostId, hostStatus, ports }: PortManagerProps) {
  const isRunning = hostStatus === "running";
  const updatePortsMutation = useUpdateHostPorts(hostId);

  const [localPorts, setLocalPorts] = useState<PortRow[]>(
    ports.length > 0
      ? ports.map((p) => ({
          host_port: String(p.host_port),
          container_port: String(p.container_port),
        }))
      : [{ host_port: "", container_port: "" }]
  );

  function updatePortRow(index: number, field: keyof PortRow, value: string) {
    const next = [...localPorts];
    next[index] = { ...next[index], [field]: value };
    setLocalPorts(next);
  }

  function removePortRow(index: number) {
    setLocalPorts(localPorts.filter((_, i) => i !== index));
  }

  function addPortRow() {
    setLocalPorts([...localPorts, { host_port: "", container_port: "" }]);
  }

  function handleSave() {
    const toSave = localPorts
      .filter(portRowValid)
      .map((p) => ({
        host_port: parseInt(p.host_port, 10),
        container_port: parseInt(p.container_port, 10),
        protocol: "tcp",
      }));
    updatePortsMutation.mutate(toSave, {
      onSuccess: () => {
        toast.success("端口映射配置已保存");
        setLocalPorts(
          toSave.length > 0
            ? toSave.map((p) => ({
                host_port: String(p.host_port),
                container_port: String(p.container_port),
              }))
            : [{ host_port: "", container_port: "" }]
        );
      },
      onError: () => toast.error("保存失败"),
    });
  }

  const hasChanges =
    JSON.stringify(
      localPorts
        .filter(portRowValid)
        .map((p) => ({
          host_port: parseInt(p.host_port, 10),
          container_port: parseInt(p.container_port, 10),
          protocol: "tcp",
        }))
    ) !== JSON.stringify(ports);

  return (
    <div className="space-y-6">
      {!isRunning && (
        <div className="space-y-3">
          <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            端口映射配置
          </p>
          <p className="text-xs text-muted-foreground">
            将宿主机端口转发到容器内部端口。外部通过"宿主机端口"访问，请求会被自动转发到容器内对应服务。
          </p>
          {localPorts.length > 0 && (
            <div className="rounded-xl border border-dashed border-border/60 bg-muted/20 p-4 space-y-3">
              {localPorts.map((p, i) => (
                <div key={i} className="flex items-end gap-2">
                  <div className="flex-1 space-y-1">
                    <span className="text-xs text-muted-foreground">宿主机端口</span>
                    <Input
                      type="number"
                      min={1}
                      max={65535}
                      placeholder="如 8080"
                      value={p.host_port}
                      onChange={(e) => updatePortRow(i, "host_port", e.target.value)}
                    />
                  </div>
                  <span className="pb-2 text-muted-foreground">:</span>
                  <div className="flex-1 space-y-1">
                    <span className="text-xs text-muted-foreground">容器端口</span>
                    <Input
                      type="number"
                      min={1}
                      max={65535}
                      placeholder="如 80"
                      value={p.container_port}
                      onChange={(e) => updatePortRow(i, "container_port", e.target.value)}
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
                        运行中主机不允许修改端口映射
                      </TooltipContent>
                    </Tooltip>
                  ) : (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="h-9 w-9 p-0 shrink-0"
                      onClick={() => removePortRow(i)}
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
              onClick={addPortRow}
            >
              <Plus className="h-4 w-4 mr-2" />
              增加端口映射
            </Button>
          )}
          {!isRunning && hasChanges && (
            <Button
              type="button"
              onClick={handleSave}
              disabled={updatePortsMutation.isPending}
            >
              {updatePortsMutation.isPending ? "保存中..." : "保存端口映射"}
            </Button>
          )}
        </div>
      )}

      {isRunning && (
        <div className="space-y-3">
          <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            已配置端口映射
          </p>
          {ports.length === 0 ? (
            <p className="py-3 text-sm text-muted-foreground">尚未配置端口映射，停止主机后可添加。</p>
          ) : (
            <ul className="space-y-3">
              {ports.map((p, i) => (
                <li
                  key={i}
                  className="flex items-center justify-between gap-3 rounded-xl border border-border/80 bg-card px-4 py-3.5 shadow-sm"
                >
                  <div className="min-w-0 flex-1">
                    <p className="font-mono text-sm leading-tight">
                      {p.host_port}:{p.container_port}
                    </p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      宿主机 {p.host_port} → 容器 {p.container_port}
                    </p>
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
