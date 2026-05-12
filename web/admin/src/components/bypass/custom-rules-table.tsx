import { useState, useMemo } from "react";
import { Pencil, Trash2, Plus, ShieldCheck, Search } from "lucide-react";
import { toast } from "sonner";
import {
  useBypassRules,
  useDeleteBypassRule,
} from "@/hooks/use-bypass-rules";
import { parseBypassError } from "@/lib/i18n/bypass-error-codes";
import type {
  BypassRule,
  BypassRuleType,
} from "@/lib/api/types/bypass";
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
import { EmptyState } from "@/components/layout/empty-state";

const TYPE_LABELS: Record<BypassRuleType, string> = {
  ip: "IP",
  cidr: "CIDR",
  domain: "域名",
  domain_suffix: "域名后缀",
  domain_keyword: "域名关键词",
};

interface CustomRulesTableProps {
  hostId: string;
}

export function CustomRulesTable({ hostId }: CustomRulesTableProps) {
  const rulesQuery = useBypassRules(hostId);
  const deleteMutation = useDeleteBypassRule(hostId);

  const [typeFilter, setTypeFilter] = useState<BypassRuleType | "all">("all");
  const [search, setSearch] = useState("");
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [drawerMode, setDrawerMode] = useState<"create" | "edit">("create");
  const [editingRule, setEditingRule] = useState<BypassRule | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<BypassRule | null>(null);

  const rules = rulesQuery.data?.rules ?? [];
  const filtered = useMemo(() => {
    return rules.filter((r) => {
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
  }, [rules, typeFilter, search]);

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

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h3 className="text-base font-semibold">自定义规则</h3>
          <p className="text-xs text-muted-foreground">
            最多 1000 条，支持 IP / CIDR / 域名 / 域名后缀 / 域名关键词 5 种类型
          </p>
        </div>
        <Button
          size="sm"
          className="gap-1.5"
          onClick={openCreate}
          data-testid="add-custom-rule"
        >
          <Plus className="size-4" />
          添加自定义规则
        </Button>
      </div>

      <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
        <div className="sm:w-40">
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
      ) : filtered.length === 0 ? (
        <EmptyState
          icon={ShieldCheck}
          title={
            rules.length === 0 ? "暂无自定义规则" : "没有匹配的规则"
          }
          description={
            rules.length === 0
              ? "当前 host 仅启用了预设规则，点击「添加自定义规则」补充域名或 IP"
              : "调整筛选条件以查看其他规则"
          }
          action={
            rules.length === 0 ? (
              <Button size="sm" onClick={openCreate} className="gap-1.5">
                <Plus className="size-4" />
                添加自定义规则
              </Button>
            ) : null
          }
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-28">类型</TableHead>
              <TableHead>值</TableHead>
              <TableHead className="w-24">风险</TableHead>
              <TableHead>备注</TableHead>
              <TableHead className="w-32 text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.map((rule) => (
              <TableRow
                key={rule.id}
                data-testid={`rules-row-${rule.id}`}
                className={
                  rule.is_risky
                    ? "border-l-2 border-l-warning hover:bg-muted/50"
                    : "hover:bg-muted/50"
                }
              >
                <TableCell>
                  <Badge variant="secondary" className="font-normal">
                    {TYPE_LABELS[rule.rule_type]}
                  </Badge>
                </TableCell>
                <TableCell className="font-mono text-sm">
                  {rule.value}
                </TableCell>
                <TableCell>
                  {rule.is_risky ? (
                    <Badge
                      data-testid="risky-badge"
                      className="bg-warning/15 text-warning-foreground hover:bg-warning/20"
                    >
                      高风险
                    </Badge>
                  ) : (
                    <span className="text-xs text-muted-foreground">—</span>
                  )}
                </TableCell>
                <TableCell className="max-w-xs truncate text-xs text-muted-foreground">
                  {rule.note || "—"}
                </TableCell>
                <TableCell className="text-right">
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-8 w-8 p-0"
                    onClick={() => openEdit(rule)}
                    aria-label="编辑规则"
                  >
                    <Pencil className="size-4" />
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
                    onClick={() => confirmDelete(rule)}
                    aria-label="删除规则"
                  >
                    <Trash2 className="size-4" />
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
              删除后白名单立即收紧。需要保存后通过「应用此配置」生效。
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
    </div>
  );
}
