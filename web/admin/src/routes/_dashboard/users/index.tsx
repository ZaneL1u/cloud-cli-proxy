import { useState } from "react";
import { createFileRoute, Link } from "@tanstack/react-router";
import { MoreHorizontal, Plus, Eye, Ban, CheckCircle, Trash2, Users } from "lucide-react";
import { toast } from "sonner";
import { useUsers, useUpdateUserStatus, type User } from "@/hooks/use-users";
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
import { CreateUserDialog } from "@/components/users/create-user-dialog";
import { DeleteUserDialog } from "@/components/users/delete-user-dialog";
import { PageHeader } from "@/components/layout/page-header";
import { DataTableShell } from "@/components/layout/data-table-shell";
import { EmptyState } from "@/components/layout/empty-state";
import { TableSkeleton } from "@/components/ui/table-skeleton";

export const Route = createFileRoute("/_dashboard/users/")({
  component: UsersPage,
});

function formatDate(dateStr: string) {
  const d = new Date(dateStr);
  return d.toLocaleDateString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function UsersPage() {
  const { data, isLoading } = useUsers();
  const updateStatus = useUpdateUserStatus();
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteUser, setDeleteUser] = useState<User | null>(null);

  const users = data?.users ?? [];

  function handleToggleStatus(user: User) {
    const newStatus = user.status === "active" ? "disabled" : "active";
    const label =
      newStatus === "active"
        ? user.status === "expired"
          ? "用户已重新激活"
          : "用户已启用"
        : "用户已禁用";
    updateStatus.mutate(
      { userId: user.id, status: newStatus },
      {
        onSuccess: () => toast.success(label),
        onError: () => toast.error("操作失败"),
      },
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="用户管理"
        description="管理系统中的所有用户账号、到期时间与登录凭证"
      >
        <Button onClick={() => setCreateOpen(true)}>
          <Plus className="h-4 w-4" />
          创建用户
        </Button>
      </PageHeader>

      <DataTableShell>
        {isLoading ? (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>用户名</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>到期时间</TableHead>
                <TableHead>创建时间</TableHead>
                <TableHead className="w-[60px]" />
              </TableRow>
            </TableHeader>
            <TableSkeleton
              rows={4}
              columns={[
                { width: "w-24" },
                { width: "w-16", pill: true },
                { width: "w-28", muted: true },
                { width: "w-32", muted: true },
                { width: "w-8", align: "right" },
              ]}
            />
          </Table>
        ) : users.length === 0 ? (
          <EmptyState
            icon={Users}
            title="暂无用户"
            description="创建第一个用户账号，即可为其分配云主机与出口 IP"
            action={
              <Button onClick={() => setCreateOpen(true)}>
                <Plus className="h-4 w-4" />
                创建用户
              </Button>
            }
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>用户名</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>到期时间</TableHead>
                <TableHead>创建时间</TableHead>
                <TableHead className="w-[60px]" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {users.map((user) => (
                <TableRow key={user.id}>
                  <TableCell>
                    <Link
                      to="/users/$userId"
                      params={{ userId: user.id }}
                      className="font-medium text-primary hover:underline"
                    >
                      {user.username}
                    </Link>
                  </TableCell>
                  <TableCell>
                    <span className="inline-flex items-center gap-2 text-sm">
                      <StatusDot
                        variant={
                          user.status === "active"
                            ? "success"
                            : user.status === "expired"
                              ? "danger"
                              : "muted"
                        }
                      />
                      {user.status === "active"
                        ? "活跃"
                        : user.status === "expired"
                          ? "已过期"
                          : "已禁用"}
                    </span>
                  </TableCell>
                  <TableCell className="text-muted-foreground tabular-nums">
                    {user.expires_at ? formatDate(user.expires_at) : "永不过期"}
                  </TableCell>
                  <TableCell className="text-muted-foreground tabular-nums">
                    {formatDate(user.created_at)}
                  </TableCell>
                  <TableCell>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon">
                          <MoreHorizontal className="h-4 w-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem asChild>
                          <Link
                            to="/users/$userId"
                            params={{ userId: user.id }}
                          >
                            <Eye />
                            查看详情
                          </Link>
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          onClick={() => handleToggleStatus(user)}
                        >
                          {user.status === "active" ? (
                            <>
                              <Ban />
                              禁用用户
                            </>
                          ) : user.status === "expired" ? (
                            <>
                              <CheckCircle />
                              重新激活
                            </>
                          ) : (
                            <>
                              <CheckCircle />
                              启用用户
                            </>
                          )}
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          variant="destructive"
                          onClick={() => setDeleteUser(user)}
                        >
                          <Trash2 />
                          删除用户
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </DataTableShell>

      <CreateUserDialog open={createOpen} onOpenChange={setCreateOpen} />
      {deleteUser && (
        <DeleteUserDialog
          user={deleteUser}
          open={!!deleteUser}
          onOpenChange={(open) => {
            if (!open) setDeleteUser(null);
          }}
        />
      )}
    </div>
  );
}
