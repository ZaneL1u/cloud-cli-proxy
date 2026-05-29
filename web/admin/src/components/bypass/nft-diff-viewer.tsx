import { useMemo } from "react";
import { ScrollArea } from "@/components/ui/scroll-area";

interface NftDiffViewerProps {
  diff: string;
}

/**
 * unified diff 渲染：
 * - `+` 行：emerald-400（增）
 * - `-` 行：red-400（删）
 * - `#` 行：slate-500（注释）
 * - 其它（context）：slate-300
 *
 * 用 `<pre>` + 文本拼接，React 默认转义，不解析 ANSI / HTML（T-46-04 mitigate）。
 */
export function NftDiffViewer({ diff }: NftDiffViewerProps) {
  const lines = useMemo(() => diff.split("\n"), [diff]);

  if (!diff.trim()) {
    return (
      <div
        data-testid="nft-diff-empty"
        className="rounded-md border bg-muted/30 py-12 text-center text-sm text-muted-foreground"
      >
        无差异（首次应用或规则集合未变更）
      </div>
    );
  }

  return (
    <ScrollArea
      data-testid="nft-diff-viewer"
      className="min-h-0 flex-1 rounded-md border bg-slate-950"
    >
      <pre className="whitespace-pre p-4 font-mono text-xs leading-relaxed">
        {lines.map((line, i) => {
          let cls = "text-slate-300";
          if (line.startsWith("+")) cls = "text-emerald-400";
          else if (line.startsWith("-")) cls = "text-red-400";
          else if (line.startsWith("#")) cls = "text-slate-500";
          return (
            <div key={i} className={cls}>
              {line || " "}
            </div>
          );
        })}
      </pre>
    </ScrollArea>
  );
}
