import { useState, useEffect, useRef, useMemo } from "react";
import { Eye, Play, RotateCcw, Shield } from "lucide-react";
import { toast } from "sonner";
import { CustomRulesTable } from "./custom-rules-table";
import { PreviewSheet } from "./preview-sheet";
import { ApplyProgressDialog } from "./apply-progress-dialog";
import { useBypassRules, useDeleteBypassRule } from "@/hooks/use-bypass-rules";
import { useBypassPresets } from "@/hooks/use-bypass-presets";
import { useBypassBindings, useCreateBypassBinding, useDeleteBypassBinding } from "@/hooks/use-bypass-bindings";
import { usePreviewBypass } from "@/hooks/use-bypass-snapshots";
import { parseBypassError } from "@/lib/i18n/bypass-error-codes";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
import type { BypassRule, BypassRuleType } from "@/lib/api/types/bypass";

interface BypassTabProps {
  hostId: string;
}

export function BypassTab({ hostId }: BypassTabProps) {
  const rulesQuery = useBypassRules(hostId);
  const presetsQuery = useBypassPresets();
  const bindingsQuery = useBypassBindings(hostId);
  const createBinding = useCreateBypassBinding(hostId);
  const deleteBinding = useDeleteBypassBinding(hostId);
  const autoBound = useRef(false);

  // 首次加载时自动绑定所有 is_force_on 的预设。
  useEffect(() => {
    if (autoBound.current) return;
    if (!presetsQuery.data?.presets || !bindingsQuery.data?.bindings) return;
    autoBound.current = true;

    const boundPresetIds = new Set(bindingsQuery.data.bindings.map((b) => b.preset_id));
    const forceOnPresets = presetsQuery.data.presets.filter((p) => p.is_force_on);
    for (const preset of forceOnPresets) {
      if (!boundPresetIds.has(preset.id)) {
        createBinding.mutate(preset.id);
      }
    }
  }, [presetsQuery.data, bindingsQuery.data, createBinding]);

  // 将已绑定预设的规则平铺展示，与自定义规则混合为统一列表。
  const boundPresets = (() => {
    if (!presetsQuery.data?.presets || !bindingsQuery.data?.bindings) return [];
    const boundPresetIds = new Set(bindingsQuery.data.bindings.map((b) => b.preset_id));
    return presetsQuery.data.presets.filter((p) => boundPresetIds.has(p.id));
  })();

  // 将预设规则展开为统一规则行（用于表格展示）。
  const presetRows: PresetRuleRow[] = boundPresets.flatMap((p) =>
    p.rules.map((r, idx) => ({
      _key: `preset-${p.id}-${idx}`,
      _kind: "preset" as const,
      rule_type: r.rule_type as BypassRuleType,
      value: r.value,
      note: r.note ?? "",
      preset_id: p.id,
      preset_name: p.name,
      binding_id: bindingsQuery.data?.bindings.find((b) => b.preset_id === p.id)?.id ?? "",
    })),
  );

  const customRules = rulesQuery.data?.rules ?? [];
  const totalRuleCount = presetRows.length + customRules.length;

  // 脏检测：对比当前规则状态与上次应用后的基线，判断是否有未生效的变更。
  const [baseline, setBaseline] = useState<{ rules: string; bindings: string } | null>(null);
  const currentFingerprint = useMemo(() => {
    const rulesStr = JSON.stringify(customRules.map((r) => `${r.rule_type}:${r.value}:${r.id}`).sort());
    const bindingsStr = JSON.stringify(bindingsQuery.data?.bindings?.map((b) => b.preset_id).sort() ?? []);
    return rulesStr + "|" + bindingsStr;
  }, [customRules, bindingsQuery.data?.bindings]);
  const isDirty = baseline !== null && currentFingerprint !== baseline.rules + "|" + baseline.bindings;

  // 应用成功后更新基线
  function markApplied() {
    setBaseline({ rules: currentFingerprint.split("|")[0], bindings: currentFingerprint.split("|")[1] });
  }

  // 首次加载完成时初始化基线
  useEffect(() => {
    if (baseline !== null) return;
    if (rulesQuery.isFetched && bindingsQuery.isFetched && presetsQuery.isFetched) {
      setBaseline({ rules: currentFingerprint.split("|")[0], bindings: currentFingerprint.split("|")[1] });
    }
  }, [baseline, rulesQuery.isFetched, bindingsQuery.isFetched, presetsQuery.isFetched, currentFingerprint]);

  const [previewOpen, setPreviewOpen] = useState(false);
  const [applyOpen, setApplyOpen] = useState(false);
  const [riskyCount, setRiskyCount] = useState(0);
  const [restoreConfirmOpen, setRestoreConfirmOpen] = useState(false);
  const deleteRule = useDeleteBypassRule(hostId);
  const previewMutation = usePreviewBypass(hostId);

  // "应用"直接下发：先拉 preview 拿 risky_count，再打开进度弹窗。
  function handleApply() {
    previewMutation.mutate(undefined, {
      onSuccess: (data) => {
        setRiskyCount(data.risky_count);
        setApplyOpen(true);
      },
      onError: (err) => {
        toast.error(parseBypassError(err).message);
      },
    });
  }

  // 恢复默认：删除所有用户自定义规则，重新绑定所有系统预设。
  function executeRestore() {
    // 1. 删除所有自定义规则
    for (const rule of customRules) {
      deleteRule.mutate(rule.id);
    }
    // 2. 重新绑定所有 is_force_on 预设
    if (presetsQuery.data?.presets && bindingsQuery.data?.bindings) {
      const boundPresetIds = new Set(bindingsQuery.data.bindings.map((b) => b.preset_id));
      const forceOnPresets = presetsQuery.data.presets.filter((p) => p.is_force_on);
      for (const preset of forceOnPresets) {
        if (!boundPresetIds.has(preset.id)) {
          createBinding.mutate(preset.id);
        }
      }
    }
    setRestoreConfirmOpen(false);
    toast.success("已恢复默认规则，请点击「应用」生效");
  }

  return (
    <div className="space-y-5" data-testid="bypass-tab">
      <header className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Shield className="size-5 text-primary" />
          <h2 className="text-base font-semibold">代理白名单</h2>
          {totalRuleCount > 0 && (
            <Badge variant="secondary" className="font-normal">
              {totalRuleCount} 条
            </Badge>
          )}
        </div>
        <div className="flex items-center gap-2">
          <Button
            data-testid="bypass-preview-button"
            size="sm"
            variant="outline"
            onClick={() => setPreviewOpen(true)}
          >
            <Eye className="mr-1.5 size-4" />
            查看配置
          </Button>
          <Button
            data-testid="bypass-restore-defaults"
            size="sm"
            variant="outline"
            onClick={() => setRestoreConfirmOpen(true)}
          >
            <RotateCcw className="mr-1.5 size-4" />
            恢复默认
          </Button>
          <Button
            data-testid="bypass-apply-button"
            size="sm"
            disabled={!isDirty || previewMutation.isPending}
            onClick={handleApply}
          >
            {previewMutation.isPending ? (
              <span className="flex items-center gap-1.5">
                <span className="size-3 animate-spin rounded-full border-2 border-current border-t-transparent" />
                加载中
              </span>
            ) : isDirty ? (
              <>
                <Play className="mr-1.5 size-4" />
                应用
                <span className="ml-1 rounded-full bg-amber-500 px-1.5 py-0 text-[10px] text-white leading-snug">未生效</span>
              </>
            ) : (
              <>
                <Play className="mr-1.5 size-4" />
                应用
              </>
            )}
          </Button>
        </div>
      </header>

      <div className="border-l-[3px] border-amber-500 bg-amber-50 px-3 py-2">
        <p className="text-xs font-semibold text-amber-800">WARNING</p>
        <p className="mt-0.5 text-xs text-amber-800/80">
          白名单内的 IP/域名将<strong className="text-amber-900">直连出网</strong>，不走代理隧道，目标服务看到的是宿主机真实 IP 而非代理出口 IP。
        </p>
      </div>

      <section>
        <CustomRulesTable
          hostId={hostId}
          presetRows={presetRows}
          onDeletePreset={(presetId) => {
            const binding = bindingsQuery.data?.bindings.find((b) => b.preset_id === presetId);
            if (binding) deleteBinding.mutate(binding.id);
          }}
        />
      </section>

      <PreviewSheet
        hostId={hostId}
        open={previewOpen}
        onOpenChange={setPreviewOpen}
      />

      <ApplyProgressDialog
        hostId={hostId}
        open={applyOpen}
        onOpenChange={setApplyOpen}
        riskyCount={riskyCount}
        onApplied={markApplied}
      />

      <AlertDialog open={restoreConfirmOpen} onOpenChange={setRestoreConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>恢复默认规则？</AlertDialogTitle>
            <AlertDialogDescription>
              将删除所有自定义规则，并恢复系统预设（本机回环、局域网等）。恢复后需点击「应用」使变更生效。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction onClick={executeRestore}>恢复</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

export interface PresetRuleRow {
  _key: string;
  _kind: "preset";
  rule_type: BypassRuleType;
  value: string;
  note: string;
  preset_id: string;
  preset_name: string;
  binding_id: string;
}
