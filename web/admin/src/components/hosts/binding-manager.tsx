import { useState } from "react";
import { Plus, Globe, ArrowRightLeft } from "lucide-react";
import { toast } from "sonner";
import {
  useBindEgressIP,
  useUnbindEgressIP,
  type HostBinding,
} from "@/hooks/use-hosts";
import { useEgressIPs } from "@/hooks/use-egress-ips";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

interface BindingManagerProps {
  hostId: string;
  hostStatus: string;
  bindings: HostBinding[];
}

export function BindingManager({
  hostId,
  hostStatus,
  bindings,
}: BindingManagerProps) {
  const isRunning = hostStatus === "running";
  const bindMutation = useBindEgressIP();
  const unbindMutation = useUnbindEgressIP();
  const { data: egressData } = useEgressIPs();
  const [selectedIpId, setSelectedIpId] = useState<string>("");

  const currentBinding = bindings[0];

  const availableIPs =
    egressData?.egress_ips?.filter((ip) => ip.status === "available") ?? [];

  const boundIpIds = new Set(bindings.map((b) => b.egress_ip.id));
  const unboundIPs = availableIPs.filter((ip) => !boundIpIds.has(ip.id));

  function handleChange() {
    if (!selectedIpId) return;
    if (!currentBinding) return;

    unbindMutation.mutate(currentBinding.binding_id, {
      onSuccess: () => {
        bindMutation.mutate(
          { host_id: hostId, egress_ip_id: selectedIpId },
          {
            onSuccess: () => {
              toast.success("出口 IP 已更换");
              setSelectedIpId("");
            },
            onError: (err) => {
              toast.error(err instanceof Error ? err.message : "新 IP 绑定失败");
            },
          },
        );
      },
      onError: (err) => {
        toast.error(err instanceof Error ? err.message : "旧 IP 解绑失败");
      },
    });
  }

  return (
    <div className="space-y-4">
      {/* 当前绑定 */}
      {currentBinding ? (
        <div className="flex items-center gap-3 rounded-lg border border-border/60 bg-muted/20 px-4 py-3">
          <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-primary/10">
            <Globe className="h-4 w-4 text-primary" />
          </div>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">
              {currentBinding.egress_ip.label}
            </p>
            <p className="font-mono text-xs text-muted-foreground">
              {currentBinding.egress_ip.ip_address}
            </p>
          </div>
        </div>
      ) : (
        <p className="py-3 text-sm text-muted-foreground">
          尚未绑定出口 IP，绑定后流量才会走指定出口。
        </p>
      )}

      {/* 更换出口 IP */}
      <div className="space-y-2">
        <p className="text-xs text-muted-foreground">
          {currentBinding
            ? "选择新的出口 IP 以更换当前绑定"
            : "选择出口 IP 进行绑定"}
        </p>
        <div className="flex flex-col gap-2 sm:flex-row sm:items-end">
          {isRunning ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="w-full flex-1">
                  <Select disabled>
                    <SelectTrigger className="h-9 w-full bg-muted/30">
                      <SelectValue placeholder="选择出口 IP" />
                    </SelectTrigger>
                  </Select>
                </span>
              </TooltipTrigger>
              <TooltipContent>运行中主机不允许更换出口 IP，请先停止主机</TooltipContent>
            </Tooltip>
          ) : (
            <Select value={selectedIpId} onValueChange={setSelectedIpId}>
              <SelectTrigger className="h-9 w-full flex-1">
                <SelectValue
                  placeholder={
                    unboundIPs.length === 0
                      ? "暂无可用的其他出口 IP"
                      : "选择新的出口 IP"
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {unboundIPs.map((ip) => (
                  <SelectItem key={ip.id} value={ip.id}>
                    {ip.label} ({ip.ip_address})
                  </SelectItem>
                ))}
                {unboundIPs.length === 0 && (
                  <SelectItem value="_none" disabled>
                    无其他可用出口 IP
                  </SelectItem>
                )}
              </SelectContent>
            </Select>
          )}
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="sm:w-auto">
                <Button
                  type="button"
                  size="sm"
                  className="h-9 w-full gap-1.5 sm:w-auto"
                  onClick={handleChange}
                  disabled={
                    isRunning ||
                    !selectedIpId ||
                    bindMutation.isPending ||
                    unbindMutation.isPending
                  }
                >
                  {currentBinding ? (
                    <ArrowRightLeft className="h-3.5 w-3.5" />
                  ) : (
                    <Plus className="h-3.5 w-3.5" />
                  )}
                  {currentBinding ? "更换" : "添加"}
                </Button>
              </span>
            </TooltipTrigger>
            {isRunning && (
              <TooltipContent>
                运行中主机不允许更换出口 IP，请先停止主机
              </TooltipContent>
            )}
          </Tooltip>
        </div>
      </div>
    </div>
  );
}
