import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import {
  MoreHorizontal,
  Plus,
  Pencil,
  Trash2,
  FlaskConical,
  Loader2,
  RefreshCw,
  Check,
  X,
  Minus,
  Globe,
} from "lucide-react";
import { toast } from "sonner";
import {
  useEgressIPs,
  useDeleteEgressIP,
  type EgressIP,
  type TestResult,
} from "@/hooks/use-egress-ips";
import { apiFetch } from "@/lib/api";
import { ApiError } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
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
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { EgressIPDrawer } from "@/components/egress-ips/egress-ip-drawer";
import { TestResultDialog } from "@/components/egress-ips/test-result-dialog";
import { egressProxyEntryDisplay } from "@/lib/egress-display";
import { PageHeader } from "@/components/layout/page-header";
import { DataTableShell } from "@/components/layout/data-table-shell";
import { EmptyState } from "@/components/layout/empty-state";

export const Route = createFileRoute("/_dashboard/egress-ips/")({
  component: EgressIPsPage,
});

const TEST_RESULTS_KEY = "egress-ip-test-results";

function loadTestResults(): Map<string, TestResult> {
  try {
    const raw = localStorage.getItem(TEST_RESULTS_KEY);
    if (raw) return new Map(JSON.parse(raw));
  } catch {
    // corrupt data
  }
  return new Map();
}

function saveTestResults(results: Map<string, TestResult>) {
  try {
    localStorage.setItem(
      TEST_RESULTS_KEY,
      JSON.stringify([...results.entries()]),
    );
  } catch {
    // quota exceeded
  }
}

function getActualIP(result: TestResult | undefined): string {
  if (!result?.results?.egress_ip) return "";
  return result.results.egress_ip.ip || "";
}

function EgressIPsPage() {
  const { data, isLoading } = useEgressIPs();
  const deleteMutation = useDeleteEgressIP();
  const [drawerMode, setDrawerMode] = useState<"create" | "edit" | null>(null);
  const [editIpId, setEditIpId] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<EgressIP | null>(null);
  const [testResults, setTestResults] =
    useState<Map<string, TestResult>>(loadTestResults);
  const [testingIds, setTestingIds] = useState<Set<string>>(new Set());
  const [testDialogResult, setTestDialogResult] = useState<TestResult | null>(
    null,
  );

  const egressIPs = data?.egress_ips ?? [];

  async function handleTest(ip: EgressIP) {
    setTestingIds((prev) => new Set(prev).add(ip.id));
    try {
      const result = await apiFetch<TestResult>(`/egress-ips/${ip.id}/test`, {
        method: "POST",
      });
      setTestResults((prev) => {
        const next = new Map(prev).set(ip.id, result);
        saveTestResults(next);
        return next;
      });
    } catch {
      toast.error(`${ip.label} 测试失败`);
    } finally {
      setTestingIds((prev) => {
        const next = new Set(prev);
        next.delete(ip.id);
        return next;
      });
    }
  }

  function handleDelete(ip: EgressIP) {
    deleteMutation.mutate(ip.id, {
      onSuccess: () => {
        toast.success("出口 IP 已删除");
        setDeleteTarget(null);
      },
      onError: (err) => {
        if (err instanceof ApiError && err.status === 409) {
          toast.error("该出口 IP 已绑定到主机，请先解绑");
        } else {
          toast.error("删除失败");
        }
        setDeleteTarget(null);
      },
    });
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="出口 IP 管理"
        description="配置代理出口，供用户主机绑定并统一出网"
      >
        <Button
          onClick={() => {
            setEditIpId(null);
            setDrawerMode("create");
          }}
        >
          <Plus className="h-4 w-4" />
          添加出口 IP
        </Button>
      </PageHeader>

      <DataTableShell>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>标签</TableHead>
              <TableHead>代理服务器</TableHead>
              <TableHead>实际出口 IP</TableHead>
              <TableHead>状态</TableHead>
              <TableHead className="w-[60px]" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              Array.from({ length: 3 }).map((_, i) => (
                <TableRow key={i}>
                  {Array.from({ length: 5 }).map((_, j) => (
                    <TableCell key={j}>
                      <div className="h-4 w-20 animate-pulse rounded bg-muted" />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : egressIPs.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} className="p-0">
                  <EmptyState
                    icon={Globe}
                    title="暂无出口 IP"
                    description="添加出口 IP 后，即可在主机上绑定并验证实际出网地址"
                    action={
                      <Button
                        onClick={() => {
                          setEditIpId(null);
                          setDrawerMode("create");
                        }}
                      >
                        <Plus className="h-4 w-4" />
                        添加出口 IP
                      </Button>
                    }
                  />
                </TableCell>
              </TableRow>
            ) : (
              egressIPs.map((ip) => {
                const result = testResults.get(ip.id);
                const actualIP = getActualIP(result);
                return (
                  <TableRow key={ip.id}>
                    <TableCell className="font-medium">{ip.label}</TableCell>
                    <TableCell className="break-all font-mono text-sm">
                      {egressProxyEntryDisplay(ip)}
                    </TableCell>
                    <TableCell>
                      {testingIds.has(ip.id) ? (
                        <span className="flex items-center gap-1.5 text-sm text-muted-foreground">
                          <Loader2 className="h-3.5 w-3.5 animate-spin" />
                          检测中…
                        </span>
                      ) : actualIP ? (
                        <span className="flex items-center gap-1.5">
                          <span className="font-mono text-sm">{actualIP}</span>
                          <button
                            onClick={() => handleTest(ip)}
                            className="rounded p-0.5 text-muted-foreground hover:text-foreground"
                            title="重新检测"
                          >
                            <RefreshCw className="h-3 w-3" />
                          </button>
                        </span>
                      ) : (
                        <button
                          onClick={() => handleTest(ip)}
                          className="flex items-center gap-1.5 text-sm text-primary hover:underline"
                        >
                          <FlaskConical className="h-3.5 w-3.5" />
                          检测
                        </button>
                      )}
                    </TableCell>
                    <TableCell>
                      <StatusCell ip={ip} result={result} onClickResult={() => result && setTestDialogResult(result)} />
                    </TableCell>
                    <TableCell>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="icon">
                            <MoreHorizontal className="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem
                            onClick={() => handleTest(ip)}
                            disabled={testingIds.has(ip.id)}
                          >
                            {testingIds.has(ip.id) ? (
                              <Loader2 className="animate-spin" />
                            ) : (
                              <FlaskConical />
                            )}
                            {testingIds.has(ip.id) ? "测试中..." : "测试"}
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            onClick={() => {
                              setEditIpId(ip.id);
                              setDrawerMode("edit");
                            }}
                          >
                            <Pencil />
                            编辑
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            variant="destructive"
                            onClick={() => setDeleteTarget(ip)}
                          >
                            <Trash2 />
                            删除
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                );
              })
            )}
          </TableBody>
        </Table>
      </DataTableShell>

      <EgressIPDrawer
        mode={drawerMode ?? "create"}
        egressIpId={editIpId}
        open={drawerMode !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDrawerMode(null);
            setEditIpId(null);
          }
        }}
        onUpdated={(ipId) => {
          setTestResults((prev) => {
            const next = new Map(prev);
            next.delete(ipId);
            saveTestResults(next);
            return next;
          });
        }}
      />

      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认删除</AlertDialogTitle>
            <AlertDialogDescription>
              确定要删除出口 IP「{deleteTarget?.label}」(
              {deleteTarget
                ? egressProxyEntryDisplay(deleteTarget)
                : ""}
              ) 吗？此操作不可撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => deleteTarget && handleDelete(deleteTarget)}
            >
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <TestResultDialog
        result={testDialogResult}
        open={testDialogResult !== null}
        onOpenChange={(open) => {
          if (!open) setTestDialogResult(null);
        }}
      />
    </div>
  );
}

function StatusCell({
  ip,
  result,
  onClickResult,
}: {
  ip: EgressIP;
  result: TestResult | undefined;
  onClickResult: () => void;
}) {
  if (ip.status === "disabled") {
    return <Badge variant="secondary">已禁用</Badge>;
  }

  if (!result) {
    return (
      <span className="flex items-center gap-1.5 text-sm text-muted-foreground">
        <Minus className="h-3.5 w-3.5" />
        待测试
      </span>
    );
  }

  const status = result.status;

  if (status === "passed") {
    return (
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              onClick={onClickResult}
              className="flex items-center gap-1.5 text-sm text-green-600 hover:underline"
            >
              <Check className="h-3.5 w-3.5" />
              正常
            </button>
          </TooltipTrigger>
          <TooltipContent>点击查看测试详情</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    );
  }

  if (status === "partial") {
    return (
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              onClick={onClickResult}
              className="flex items-center gap-1.5 text-sm text-yellow-600 hover:underline"
            >
              <X className="h-3.5 w-3.5" />
              部分异常
            </button>
          </TooltipTrigger>
          <TooltipContent>点击查看测试详情</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    );
  }

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            onClick={onClickResult}
            className="flex items-center gap-1.5 text-sm text-destructive hover:underline"
          >
            <X className="h-3.5 w-3.5" />
            异常
          </button>
        </TooltipTrigger>
        <TooltipContent>
          {result.message || "点击查看测试详情"}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
