import { useState } from "react";
import { toast } from "sonner";
import { useHostAction } from "@/hooks/use-hosts";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";

interface RebuildDialogProps {
  hostId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function RebuildDialog({
  hostId,
  open,
  onOpenChange,
}: RebuildDialogProps) {
  const [mode, setMode] = useState<"preserve" | "factory">("preserve");
  const actionMutation = useHostAction();

  function handleRebuild() {
    actionMutation.mutate(
      { hostId, action: "rebuild", body: { mode } },
      {
        onSuccess: () => {
          toast.success("重建操作已提交，请查看任务状态");
          onOpenChange(false);
        },
        onError: () => toast.error("重建操作提交失败"),
      },
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>重建主机</DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div className="space-y-3">
            <Label className="text-sm font-medium">选择重建模式</Label>

            <label className="flex cursor-pointer items-start gap-3 rounded-md border p-3 has-[:checked]:border-primary has-[:checked]:bg-accent">
              <input
                type="radio"
                name="rebuild-mode"
                value="preserve"
                checked={mode === "preserve"}
                onChange={() => setMode("preserve")}
                className="mt-1"
              />
              <div>
                <div className="font-medium">保留主目录并重置系统层</div>
                <div className="text-sm text-muted-foreground">
                  保留用户的 home 目录数据，仅重置系统环境和预装工具
                </div>
              </div>
            </label>

            <label className="flex cursor-pointer items-start gap-3 rounded-md border p-3 has-[:checked]:border-primary has-[:checked]:bg-accent">
              <input
                type="radio"
                name="rebuild-mode"
                value="factory"
                checked={mode === "factory"}
                onChange={() => setMode("factory")}
                className="mt-1"
              />
              <div>
                <div className="font-medium">工厂重置（清除所有数据）</div>
                <div className="text-sm text-muted-foreground">
                  清除所有用户数据并恢复至初始状态
                </div>
              </div>
            </label>

            <div className="rounded-md border bg-muted/50 p-3 text-xs text-muted-foreground space-y-1.5">
              <p className="font-medium text-foreground text-sm">重建影响说明</p>
              <p><strong>保留（不受影响）：</strong></p>
              <ul className="list-disc pl-4 space-y-0.5">
                <li>home 目录（/workspace）下的所有文件{mode === "factory" ? "（工厂重置会清除）" : ""}</li>
                <li>SSH 密钥（由平台管理，重建后自动重新注入）</li>
                <li>SSH / 登录密码（平台侧存储）</li>
              </ul>
              <p><strong>会被清除：</strong></p>
              <ul className="list-disc pl-4 space-y-0.5">
                <li>容器系统层（通过 apt 安装的额外软件包）</li>
                <li>系统级配置修改（/etc/ 下的变更）</li>
                <li>/tmp、/var 等非持久目录</li>
              </ul>
            </div>

            {mode === "factory" && (
              <div className="rounded-md border border-destructive/50 bg-destructive/5 p-3 text-sm text-destructive">
                工厂重置将清除 /workspace 目录中的所有数据（包括 .ssh 密钥、浏览器缓存、Claude 登录态等），不可恢复。SSH 密钥会从平台重新注入。
              </div>
            )}
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button
            onClick={handleRebuild}
            disabled={actionMutation.isPending}
            variant={mode === "factory" ? "destructive" : "default"}
          >
            {actionMutation.isPending ? "提交中..." : "确认重建"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
