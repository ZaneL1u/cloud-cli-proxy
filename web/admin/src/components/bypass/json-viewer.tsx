import { useMemo, useState } from "react";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Button } from "@/components/ui/button";

interface JSONViewerProps {
  value: unknown;
}

/**
 * 只读 JSON 展示器。
 *
 * UI-SPEC 约束：
 * - 当 stringify 后超过 10000 行时，默认折叠并显示「展开完整内容」按钮，
 *   防止巨型 sing-box rule-set 卡死浏览器。
 * - 渲染用 `<pre>` + `JSON.stringify`，不解析 HTML，避免 XSS（T-46-04）。
 */
export function JSONViewer({ value }: JSONViewerProps) {
  const text = useMemo(() => JSON.stringify(value, null, 2), [value]);
  const lineCount = useMemo(() => text.split("\n").length, [text]);
  const [expanded, setExpanded] = useState(false);
  const needsFold = lineCount > 10000;

  if (needsFold && !expanded) {
    return (
      <div
        data-testid="json-viewer-fold"
        className="rounded-md border bg-muted/30 p-6 text-center"
      >
        <p className="mb-3 text-sm text-muted-foreground">
          配置超过 1 万行（{lineCount.toLocaleString()} 行），点击展开完整内容（可能影响浏览器性能）
        </p>
        <Button size="sm" variant="outline" onClick={() => setExpanded(true)}>
          展开完整内容
        </Button>
      </div>
    );
  }

  return (
    <ScrollArea
      data-testid="json-viewer"
      className="min-h-0 flex-1 rounded-md border bg-slate-950"
    >
      <pre className="whitespace-pre p-4 font-mono text-xs leading-relaxed text-slate-100">
        {text}
      </pre>
    </ScrollArea>
  );
}
