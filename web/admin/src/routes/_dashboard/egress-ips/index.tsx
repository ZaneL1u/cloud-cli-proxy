import { useState, useEffect } from "react";
import { createFileRoute } from "@tanstack/react-router";
import {
  MoreHorizontal,
  Plus,
  Pencil,
  Trash2,
  FlaskConical,
  Loader2,
  RefreshCw,
  Globe,
} from "lucide-react";
import { toast } from "sonner";
import {
  useEgressIPs,
  useDeleteEgressIP,
  useTestEgressIPSSE,
  type EgressIP,
  type TestResult,
  type ProbeStage,
} from "@/hooks/use-egress-ips";
import { Button } from "@/components/ui/button";
import { StatusDot } from "@/components/ui/status-dot";
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
import { TableSkeleton } from "@/components/ui/table-skeleton";

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

function stageLabel(stage: ProbeStage | null): string {
  switch (stage) {
    case "pulling":
      return "拉取镜像";
    case "starting":
      return "初始化容器";
    case "connecting":
      return "建立连接";
    case "testing":
      return "执行检测";
    default:
      return "检测中…";
  }
}

function EgressIPsPage() {
  const { data, isLoading } = useEgressIPs();
  const deleteMutation = useDeleteEgressIP();
  const sseTest = useTestEgressIPSSE();
  const [drawerMode, setDrawerMode] = useState<"create" | "edit" | null>(null);
  const [editIpId, setEditIpId] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<EgressIP | null>(null);
  const [testResults, setTestResults] =
    useState<Map<string, TestResult>>(loadTestResults);
  const [testDialogIpId, setTestDialogIpId] = useState<string | null>(null);

  const egressIPs = data?.egress_ips ?? [];

  // 当任意 SSE 流完成时，保存结果到 localStorage
  useEffect(() => {
    for (const [ipId, state] of sseTest.states) {
      if (state.stage === "done" && state.result) {
        setTestResults((prev) => {
          if (prev.has(ipId)) return prev;
          const next = new Map(prev).set(ipId, state.result!);
          saveTestResults(next);
          return next;
        });
      }
    }
  }, [sseTest.states]);

  function handleTest(ip: EgressIP) {
    setTestDialogIpId(ip.id);
    sseTest.start(ip.id);
  }

  function handleDelete(ip: EgressIP) {
    deleteMutation.mutate(ip.id, {
      onSuccess: () => {
        toast.success("出口 IP 已删除");
        setDeleteTarget(null);
      },
      onError: (err: Error) => {
        // @ts-expect-error ApiError may have status
        if (err.status === 409) {
          toast.error("该出口 IP 已绑定到主机，请先解绑");
        } else {
          toast.error("删除失败");
        }
        setDeleteTarget(null);
      },
    });
  }

  const dialogState = testDialogIpId
    ? sseTest.states.get(testDialogIpId)
    : undefined;

  // 弹窗只在点击已有结果时打开，检测过程中不自动弹窗
  const dialogOpen =
    testDialogIpId !== null &&
    !(dialogState?.isRunning ?? false) &&
    testResults.get(testDialogIpId) != null;

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
        {isLoading ? (
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
            <TableSkeleton
              rows={4}
              columns={[
                { width: "w-24" },
                { width: "w-48" },
                { width: "w-28" },
                { width: "w-20", pill: true },
                { width: "w-8", align: "right" },
              ]}
            />
          </Table>
        ) : egressIPs.length === 0 ? (
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
        ) : (
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
              {egressIPs.map((ip) => {
                const result = testResults.get(ip.id);
                const actualIP = getActualIP(result);
                const testState = sseTest.states.get(ip.id);
                const isTestingThis = testState?.isRunning ?? false;
                const stage = testState?.stage ?? null;
                return (
                  <TableRow key={ip.id}>
                    <TableCell className="font-medium">{ip.label}</TableCell>
                    <TableCell className="break-all font-mono text-sm">
                      {egressProxyEntryDisplay(ip)}
                    </TableCell>
                    <TableCell>
                      {isTestingThis ? (
                        <span className="text-sm text-muted-foreground">
                          —
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
                      <StatusCell
                        ip={ip}
                        result={result}
                        isTesting={isTestingThis}
                        stage={stage}
                        onClickResult={() => {
                          if (result) {
                            setTestDialogIpId(ip.id);
                          }
                        }}
                      />
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
                            disabled={isTestingThis}
                          >
                            {isTestingThis ? (
                              <>
                                <Loader2 className="animate-spin" />
                                检测中…
                              </>
                            ) : (
                              <>
                                <FlaskConical />
                                测试
                              </>
                            )}
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
              })}
            </TableBody>
          </Table>
        )}
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
        result={
          dialogState?.stage === "done" || dialogState?.stage === "error"
            ? (dialogState?.result ?? null)
            : testDialogIpId
              ? testResults.get(testDialogIpId) ?? null
              : null
        }
        stage={dialogState?.stage ?? null}
        message={dialogState?.message ?? ""}
        open={dialogOpen}
        onOpenChange={(open) => {
          if (!open) {
            if (testDialogIpId) sseTest.stop(testDialogIpId);
            setTestDialogIpId(null);
          }
        }}
      />
    </div>
  );
}

function StatusCell({
  ip,
  result,
  isTesting,
  stage,
  onClickResult,
}: {
  ip: EgressIP;
  result: TestResult | undefined;
  isTesting?: boolean;
  stage?: ProbeStage | null;
  onClickResult: () => void;
}) {
  if (ip.status === "disabled") {
    return (
      <span className="flex items-center gap-2 text-sm text-muted-foreground">
        <StatusDot variant="muted" />
        已禁用
      </span>
    );
  }

  if (isTesting) {
    return (
      <span className="flex items-center gap-1.5 text-sm text-info">
        <Loader2 className="h-3.5 w-3.5 animate-spin" />
        {stageLabel(stage)}
      </span>
    );
  }

  if (!result) {
    return (
      <span className="flex items-center gap-2 text-sm text-muted-foreground">
        <StatusDot variant="muted" />
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
              className="flex items-center gap-2 text-sm text-success hover:underline"
            >
              <StatusDot variant="success" />
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
              className="flex items-center gap-2 text-sm text-warning hover:underline"
            >
              <StatusDot variant="warning" />
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
            className="flex items-center gap-2 text-sm text-destructive hover:underline"
          >
            <StatusDot variant="danger" />
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
