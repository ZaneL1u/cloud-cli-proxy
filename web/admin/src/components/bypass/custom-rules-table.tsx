import { useMemo, useRef, useState } from "react";
import { Download, Pencil, Plus, Search, Trash2, Upload } from "lucide-react";
import { toast } from "sonner";
import {
  useBypassRules,
  useCreateBypassRule,
  useDeleteBypassRule,
} from "@/hooks/use-bypass-rules";
import { parseBypassError } from "@/lib/i18n/bypass-error-codes";
import type {
  BypassRule,
  BypassRuleType,
} from "@/lib/api/types/bypass";
import type { PresetRuleRow } from "./bypass-tab";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
import { BypassRuleDrawer } from "./bypass-rule-drawer";

const TYPE_LABELS: Record<BypassRuleType, string> = {
  ip: "IP",
  cidr: "CIDR",
  domain: "域名",
  domain_suffix: "域名后缀",
  domain_keyword: "域名关键词",
};

const VALID_RULE_TYPES = new Set<BypassRuleType>([
  "ip",
  "cidr",
  "domain",
  "domain_suffix",
  "domain_keyword",
]);

interface BypassRulesExportFile {
  version: number;
  exported_at: string;
  rules: Array<{
    rule_type: BypassRuleType;
    value: string;
    note?: string;
  }>;
}

interface CustomRulesTableProps {
  hostId: string;
  presetRows?: PresetRuleRow[];
  onDeletePreset?: (presetId: string) => void;
}

export function CustomRulesTable({ hostId, presetRows, onDeletePreset }: CustomRulesTableProps) {
  const rulesQuery = useBypassRules(hostId);
  const createMutation = useCreateBypassRule(hostId);
  const deleteMutation = useDeleteBypassRule(hostId);
  const importInputRef = useRef<HTMLInputElement>(null);

  const [typeFilter, setTypeFilter] = useState<BypassRuleType | "all">("all");
  const [search, setSearch] = useState("");
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [drawerMode, setDrawerMode] = useState<"create" | "edit">("create");
  const [editingRule, setEditingRule] = useState<BypassRule | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<BypassRule | null>(null);
  const [deletePresetTarget, setDeletePresetTarget] = useState<PresetRuleRow | null>(null);

  const customRules = rulesQuery.data?.rules ?? [];
  const exportableRules = useMemo(
    () => [
      ...(presetRows?.map((rule) => ({
        rule_type: rule.rule_type,
        value: rule.value,
        note: rule.note || undefined,
      })) ?? []),
      ...customRules.map((rule) => ({
        rule_type: rule.rule_type,
        value: rule.value,
        note: rule.note ?? undefined,
      })),
    ],
    [customRules, presetRows],
  );

  const filtered = useMemo(() => {
    const all = customRules.filter((r) => {
      if (typeFilter !== "all" && r.rule_type !== typeFilter) return false;
      if (search) {
        const q = search.toLowerCase();
        if (
          !r.value.toLowerCase().includes(q) &&
          !(r.note ?? "").toLowerCase().includes(q)
        ) {
          return false;
        }
      }
      return true;
    });
    return all;
  }, [customRules, typeFilter, search]);

  const filteredPresets = useMemo(() => {
    if (!presetRows) return [];
    return presetRows.filter((pr) => {
      if (typeFilter !== "all" && pr.rule_type !== typeFilter) return false;
      if (search) {
        const q = search.toLowerCase();
        if (
          !pr.value.toLowerCase().includes(q) &&
          !(pr.note ?? "").toLowerCase().includes(q)
        ) {
          return false;
        }
      }
      return true;
    });
  }, [presetRows, typeFilter, search]);

  function openCreate() {
    setDrawerMode("create");
    setEditingRule(null);
    setDrawerOpen(true);
  }

  function openEdit(rule: BypassRule) {
    setDrawerMode("edit");
    setEditingRule(rule);
    setDrawerOpen(true);
  }

  function confirmDelete(rule: BypassRule) {
    setDeleteTarget(rule);
  }

  function executeDelete() {
    if (!deleteTarget) return;
    deleteMutation.mutate(deleteTarget.id, {
      onSuccess: () => {
        toast.success("规则已删除");
        setDeleteTarget(null);
      },
      onError: (err) => toast.error(parseBypassError(err).message),
    });
  }

  function confirmDeletePreset(pr: PresetRuleRow) {
    setDeletePresetTarget(pr);
  }

  function executeDeletePreset() {
    if (!deletePresetTarget) return;
    onDeletePreset?.(deletePresetTarget.preset_id);
    toast.success("预设规则已移除");
    setDeletePresetTarget(null);
  }

  async function handleExportRules() {
    if (exportableRules.length === 0) {
      toast.info("暂无可导出的规则");
      return;
    }

    const data: BypassRulesExportFile = {
      version: 1,
      exported_at: new Date().toISOString(),
      rules: exportableRules,
    };
    const blob = new Blob([JSON.stringify(data, null, 2)], {
      type: "application/json",
    });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `bypass-rules-${hostId}.json`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
    toast.success("规则已导出");
  }

  async function handleImportRules(file: File) {
    try {
      const text = await file.text();
      const parsed = JSON.parse(text) as Partial<BypassRulesExportFile> | BypassRulesExportFile["rules"];
      const rules = Array.isArray(parsed) ? parsed : parsed.rules;

      if (!Array.isArray(rules) || rules.length === 0) {
        toast.error("导入文件中没有规则");
        return;
      }

      for (const rule of rules) {
        if (
          !rule ||
          !VALID_RULE_TYPES.has(rule.rule_type) ||
          typeof rule.value !== "string" ||
          rule.value.trim() === ""
        ) {
          toast.error("导入文件格式不正确");
          return;
        }
      }

      await Promise.all(
        rules.map((rule) =>
          createMutation.mutateAsync({
            rule_type: rule.rule_type,
            value: rule.value.trim(),
            note: typeof rule.note === "string" ? rule.note : undefined,
            confirm_risky: true,
          }),
        ),
      );
      toast.success(`已导入 ${rules.length} 条规则，请点击「应用」生效`);
    } catch (err) {
      if (err instanceof SyntaxError) {
        toast.error("导入文件不是有效 JSON");
        return;
      }
      toast.error(parseBypassError(err).message);
    } finally {
      if (importInputRef.current) {
        importInputRef.current.value = "";
      }
    }
  }

  const hasAny = (presetRows?.length ?? 0) + customRules.length > 0;
  const filteredTotal = filteredPresets.length + filtered.length;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <h3 className="text-base font-semibold">规则</h3>
        <div className="flex items-center gap-2">
          <Button
            size="sm"
            className="gap-1.5"
            onClick={openCreate}
            data-testid="add-custom-rule"
          >
            <Plus className="size-4" />
            添加规则
          </Button>
          <Button
            size="sm"
            variant="outline"
            className="gap-1.5"
            onClick={handleExportRules}
            disabled={exportableRules.length === 0}
            data-testid="export-custom-rules"
          >
            <Download className="size-4" />
            规则导出
          </Button>
          <Button
            size="sm"
            variant="outline"
            className="gap-1.5"
            onClick={() => importInputRef.current?.click()}
            disabled={createMutation.isPending}
            data-testid="import-custom-rules"
          >
            <Upload className="size-4" />
            规则导入
          </Button>
          <input
            ref={importInputRef}
            type="file"
            accept="application/json,.json"
            className="hidden"
            onChange={(event) => {
              const file = event.target.files?.[0];
              if (file) void handleImportRules(file);
            }}
            data-testid="import-custom-rules-input"
          />
        </div>
      </div>

      <div className="flex items-center gap-2">
        <div className="w-32">
          <Select
            value={typeFilter}
            onValueChange={(v) => setTypeFilter(v as BypassRuleType | "all")}
          >
            <SelectTrigger>
              <SelectValue placeholder="全部类型" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部类型</SelectItem>
              {(
                [
                  "ip",
                  "cidr",
                  "domain",
                  "domain_suffix",
                  "domain_keyword",
                ] as BypassRuleType[]
              ).map((t) => (
                <SelectItem key={t} value={t}>
                  {TYPE_LABELS[t]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="relative flex-1">
          <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="搜索值或备注…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-8"
          />
        </div>
      </div>

      {rulesQuery.isLoading ? (
        <div className="space-y-2">
          {[0, 1, 2].map((i) => (
            <div
              key={i}
              className="h-10 animate-pulse rounded bg-muted"
              data-testid="rules-row-skeleton"
            />
          ))}
        </div>
      ) : !hasAny ? (
        <div className="flex flex-col items-center gap-3 py-10 text-center">
          <p className="text-sm text-muted-foreground">暂无规则</p>
          <Button size="sm" onClick={openCreate} className="gap-1.5">
            <Plus className="size-4" />
            添加规则
          </Button>
        </div>
      ) : filteredTotal === 0 ? (
        <div className="py-8 text-center text-sm text-muted-foreground">
          没有匹配的规则，调整筛选条件
        </div>
      ) : (
        <Table className="text-sm">
          <TableHeader>
            <TableRow>
              <TableHead className="h-8 w-24">类型</TableHead>
              <TableHead className="h-8">值</TableHead>
              <TableHead className="h-8">备注</TableHead>
              <TableHead className="h-8 w-24 text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filteredPresets.map((pr) => (
              <TableRow key={pr._key} className="h-8 hover:bg-muted/50">
                <TableCell className="py-1">
                  <Badge variant="secondary" className="px-1.5 py-0 text-[11px] font-normal">
                    {TYPE_LABELS[pr.rule_type]}
                  </Badge>
                </TableCell>
                <TableCell className="py-1 font-mono text-[13px]">{pr.value}</TableCell>
                <TableCell className="py-1 max-w-xs truncate text-[12px] text-muted-foreground">
                  {pr.note || "—"}
                </TableCell>
                <TableCell className="py-1 text-right">
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-6 w-6 p-0 text-muted-foreground hover:text-destructive"
                    onClick={() => confirmDeletePreset(pr)}
                    aria-label="移除预设规则"
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
            {filtered.map((rule) => (
              <TableRow
                key={rule.id}
                data-testid={`rules-row-${rule.id}`}
                className="h-8 hover:bg-muted/50"
              >
                <TableCell className="py-1">
                  <Badge variant="secondary" className="px-1.5 py-0 text-[11px] font-normal">
                    {TYPE_LABELS[rule.rule_type]}
                  </Badge>
                </TableCell>
                <TableCell className="py-1 font-mono text-[13px]">
                  {rule.value}
                </TableCell>
                <TableCell className="py-1 max-w-xs truncate text-[12px] text-muted-foreground">
                  {rule.note || "—"}
                </TableCell>
                <TableCell className="py-1 text-right">
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-6 w-6 p-0"
                    onClick={() => openEdit(rule)}
                    aria-label="编辑规则"
                  >
                    <Pencil className="size-3.5" />
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-6 w-6 p-0 text-muted-foreground hover:text-destructive"
                    onClick={() => confirmDelete(rule)}
                    aria-label="删除规则"
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <BypassRuleDrawer
        hostId={hostId}
        mode={drawerMode}
        open={drawerOpen}
        onOpenChange={setDrawerOpen}
        rule={editingRule}
      />

      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(o) => !o && setDeleteTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除该规则？</AlertDialogTitle>
            <AlertDialogDescription>
              删除后需点击「应用」使变更生效。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={executeDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={!!deletePresetTarget}
        onOpenChange={(o) => !o && setDeletePresetTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>移除预设规则？</AlertDialogTitle>
            <AlertDialogDescription>
              移除后可通过「恢复默认」重新添加。移除后需点击「应用」使变更生效。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={executeDeletePreset}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              移除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
