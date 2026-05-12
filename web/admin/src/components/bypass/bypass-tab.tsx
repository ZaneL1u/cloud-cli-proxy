import { Shield } from "lucide-react";
import { PresetGrid } from "./preset-grid";
import { CustomRulesTable } from "./custom-rules-table";
import { useBypassRules } from "@/hooks/use-bypass-rules";
import { Badge } from "@/components/ui/badge";

interface BypassTabProps {
  hostId: string;
}

/**
 * 代理白名单 Tab 顶层容器。
 * 结构：标题（含规则数 Badge）→ 预设区 → 自定义规则表。
 * 预览面板 / 应用进度 / Rollback 等子流程由后续 plan（46-04/05）补齐。
 */
export function BypassTab({ hostId }: BypassTabProps) {
  const rulesQuery = useBypassRules(hostId);
  const ruleCount = rulesQuery.data?.rules.length ?? 0;

  return (
    <div className="space-y-6" data-testid="bypass-tab">
      <header className="flex items-center gap-2">
        <Shield className="size-5 text-primary" />
        <h2 className="text-base font-semibold">代理白名单</h2>
        {ruleCount > 0 && (
          <Badge variant="secondary" className="font-normal">
            {ruleCount} 条规则
          </Badge>
        )}
      </header>

      <section className="space-y-3">
        <div>
          <h3 className="text-base font-semibold">预设规则集</h3>
          <p className="text-xs text-muted-foreground">
            选中预设以快速启用一组系统维护的白名单规则
          </p>
        </div>
        <PresetGrid hostId={hostId} />
      </section>

      <section>
        <CustomRulesTable hostId={hostId} />
      </section>
    </div>
  );
}
