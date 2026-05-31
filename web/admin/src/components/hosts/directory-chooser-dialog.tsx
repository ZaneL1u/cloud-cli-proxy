import { useState, useEffect, useCallback } from "react";
import {
  Folder,
  File,
  ChevronRight,
  Home,
  FolderOpen,
  Loader2,
  AlertCircle,
} from "lucide-react";
import { useHostFiles } from "@/hooks/use-host-files";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  buildBreadcrumbs,
  isWindowsPath,
  toDockerMountPath,
  getParentPath,
} from "@/lib/path-utils";

function formatFileSize(bytes: number): string {
  if (bytes >= 1024 * 1024) {
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }
  if (bytes >= 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`;
  }
  return `${bytes} B`;
}

function formatModTime(dateStr: string): string {
  if (!dateStr) return "—";
  try {
    return new Date(dateStr).toLocaleDateString("zh-CN", {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return "—";
  }
}

interface DirectoryChooserDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (path: string) => void;
  initialPath?: string;
}

function resolveInitialPath(initialPath?: string): string {
  if (initialPath && initialPath.length > 0) {
    if (isWindowsPath(initialPath)) {
      const sep = initialPath.includes("\\") ? "\\" : "/";
      const drive = initialPath.substring(0, 2) + sep;
      const rest = initialPath.slice(2).replace(/[\\/]+/g, sep).replace(/^[\\/]/, "");
      if (!rest) return drive;
      let result = drive;
      const parts = rest.split(sep).filter(Boolean);
      for (let i = 0; i < parts.length; i++) {
        result += parts[i] + sep;
      }
      return result;
    }
    const normalized = initialPath.replace(/\/+$/, "");
    if (normalized === "") return "/";
    return normalized.endsWith("/") ? normalized : normalized + "/";
  }
  return "/";
}

export function DirectoryChooserDialog({
  open,
  onOpenChange,
  onSelect,
  initialPath,
}: DirectoryChooserDialogProps) {
  const [currentPath, setCurrentPath] = useState(() =>
    resolveInitialPath(initialPath),
  );

  useEffect(() => {
    if (open) {
      setCurrentPath(resolveInitialPath(initialPath));
    }
  }, [open, initialPath]);

  const { data, isLoading, isError, refetch } = useHostFiles(currentPath);
  const entries = data?.entries ?? [];
  const breadcrumbs = buildBreadcrumbs(currentPath);
  const dockerPath = isWindowsPath(currentPath)
    ? toDockerMountPath(currentPath)
    : null;

  const handleSelect = useCallback(() => {
    onSelect(toDockerMountPath(currentPath));
    onOpenChange(false);
  }, [currentPath, onSelect, onOpenChange]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Backspace") {
        e.preventDefault();
        const parent = getParentPath(currentPath);
        if (parent !== currentPath) {
          setCurrentPath(parent);
        }
      }
    },
    [currentPath],
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="sm:max-w-2xl"
        showCloseButton={false}
        onKeyDown={handleKeyDown}
      >
        <DialogHeader>
          <DialogTitle>选择目录</DialogTitle>
        </DialogHeader>

        <div className="space-y-3">
          {/* 面包屑导航 */}
          <nav
            className="flex flex-wrap items-center gap-1 text-sm"
            aria-label="面包屑导航"
          >
            {breadcrumbs.map((crumb, index) => (
              <span key={crumb.path} className="flex items-center gap-1">
                {index > 0 && (
                  <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                )}
                {index === 0 ? (
                  <button
                    type="button"
                    onClick={() => setCurrentPath(crumb.path)}
                    className="flex items-center gap-1 rounded px-1.5 py-0.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                  >
                    <Home className="h-3.5 w-3.5" />
                    {crumb.label}
                  </button>
                ) : index === breadcrumbs.length - 1 ? (
                  <span className="flex items-center gap-1 rounded px-1.5 py-0.5 font-medium text-foreground">
                    <FolderOpen className="h-3.5 w-3.5" />
                    {crumb.label}
                  </span>
                ) : (
                  <button
                    type="button"
                    onClick={() => setCurrentPath(crumb.path)}
                    className="rounded px-1.5 py-0.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                  >
                    {crumb.label}
                  </button>
                )}
              </span>
            ))}
          </nav>

          {/* Docker 挂载路径预览 */}
          {dockerPath && (
            <div className="rounded-md border border-blue-200 bg-blue-50 px-3 py-2 dark:border-blue-800 dark:bg-blue-950">
              <span className="text-xs text-muted-foreground">
                Docker 挂载路径：
              </span>
              <code className="ml-1 font-mono text-sm">{dockerPath}</code>
            </div>
          )}

          {/* 目录列表 */}
          <ScrollArea className="h-[360px] rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>名称</TableHead>
                  <TableHead className="w-[100px]">大小</TableHead>
                  <TableHead className="w-[160px]">修改时间</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {isLoading ? (
                  Array.from({ length: 8 }).map((_, i) => (
                    <TableRow key={i}>
                      {Array.from({ length: 3 }).map((_, j) => (
                        <TableCell key={j}>
                          <div className="h-4 w-20 animate-pulse rounded bg-muted" />
                        </TableCell>
                      ))}
                    </TableRow>
                  ))
                ) : isError ? (
                  <TableRow>
                    <TableCell colSpan={3}>
                      <div className="flex flex-col items-center gap-2 py-12">
                        <AlertCircle className="h-8 w-8 text-destructive" />
                        <p className="text-sm text-muted-foreground">
                          加载失败，请检查权限后重试
                        </p>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => refetch()}
                        >
                          重试
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ) : entries.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={3}>
                      <div className="flex flex-col items-center gap-2 py-12">
                        <FolderOpen className="h-8 w-8 text-muted-foreground" />
                        <p className="text-sm text-muted-foreground">
                          当前目录为空
                        </p>
                      </div>
                    </TableCell>
                  </TableRow>
                ) : (
                  entries.map((entry) => (
                    <TableRow key={entry.path}>
                      <TableCell>
                        {entry.is_dir ? (
                          <button
                            type="button"
                            onClick={() => {
                              const sep = currentPath.includes("\\")
                                ? "\\"
                                : "/";
                              setCurrentPath(
                                currentPath.replace(/[\\/]+$/, "") +
                                  sep +
                                  entry.name,
                              );
                            }}
                            className="flex items-center gap-2 text-left transition-colors hover:text-primary"
                          >
                            <Folder className="h-4 w-4 shrink-0 text-blue-500" />
                            <span className="font-medium">{entry.name}</span>
                          </button>
                        ) : (
                          <span className="flex items-center gap-2 text-muted-foreground">
                            <File className="h-4 w-4 shrink-0" />
                            <span>{entry.name}</span>
                          </span>
                        )}
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        {entry.is_dir ? "—" : formatFileSize(entry.size)}
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        {formatModTime(entry.mod_time)}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </ScrollArea>
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            取消
          </Button>
          <Button onClick={handleSelect}>选择此目录</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
