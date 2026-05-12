import { useState, useEffect } from "react";
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

interface RiskyKeywordConfirmProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  keyword: string;
  onConfirm: () => void;
}

/**
 * `domain_keyword < 4` 字符的二次确认。
 * 用户必须勾选「我已知悉」复选框才能点击「仍要保存」。
 * 复用 shadcn AlertDialog（46-UI-SPEC 已锁定该 primitive 已经存在）。
 */
export function RiskyKeywordConfirm({
  open,
  onOpenChange,
  keyword,
  onConfirm,
}: RiskyKeywordConfirmProps) {
  const [ack, setAck] = useState(false);

  // 每次打开重置勾选状态，避免上次状态串扰
  useEffect(() => {
    if (open) setAck(false);
  }, [open]);

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle className="text-warning-foreground">
            关键词较短，存在误命中风险
          </AlertDialogTitle>
          <AlertDialogDescription>
            关键词{" "}
            <span className="font-mono text-warning-foreground">
              「{keyword}」
            </span>{" "}
            少于 4 字符，可能误命中其他域名（例如{" "}
            <span className="font-mono">{keyword}.com</span> /{" "}
            <span className="font-mono">{keyword}def.org</span>）。建议使用更长更具体的关键词。
          </AlertDialogDescription>
        </AlertDialogHeader>

        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            data-testid="risky-ack"
            checked={ack}
            onChange={(e) => setAck(e.target.checked)}
            className="size-4 rounded border-input accent-warning"
          />
          <span>我已知悉该关键词可能误命中其他域名</span>
        </label>

        <AlertDialogFooter>
          <AlertDialogCancel>取消</AlertDialogCancel>
          <AlertDialogAction
            disabled={!ack}
            onClick={(e) => {
              if (!ack) {
                e.preventDefault();
                return;
              }
              onConfirm();
            }}
            className="bg-warning text-warning-foreground hover:bg-warning/90 disabled:opacity-50"
          >
            仍要保存
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
