import { useState } from "react";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import {
  ArrowLeft,
  Ban,
  Calendar,
  CheckCircle,
  Copy,
  KeyRound,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { useUser, useUpdateUserStatus, useUpdateUserExpiry } from "@/hooks/use-users";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { DeleteUserDialog } from "@/components/users/delete-user-dialog";
import { RotatePasswordDialog } from "@/components/users/rotate-password-dialog";
import { SSHKeyManager } from "@/components/ssh-keys/ssh-key-manager";
import {
  useAdminSSHKeys,
  useAdminGenerateSSHKey,
  useAdminSetSSHKey,
  useAdminDeleteSSHKey,
} from "@/hooks/use-ssh-keys";

export const Route = createFileRoute("/_dashboard/users/$userId")({
  component: UserDetailPage,
});

function formatDate(dateStr: string) {
  return new Date(dateStr).toLocaleDateString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function UserDetailPage() {
  const { userId } = Route.useParams();
  const navigate = useNavigate();
  const { data, isLoading } = useUser(userId);
  const updateStatus = useUpdateUserStatus();
  const updateExpiry = useUpdateUserExpiry();
  const [rotateOpen, setRotateOpen] = useState(false);
  const [shortIdCopied, setShortIdCopied] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [expiryOpen, setExpiryOpen] = useState(false);
  const [expiryValue, setExpiryValue] = useState("");
  const sshKeysQuery = useAdminSSHKeys(userId);
  const generateSSHKey = useAdminGenerateSSHKey();
  const setSSHKey = useAdminSetSSHKey();
  const deleteSSHKey = useAdminDeleteSSHKey();

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="h-8 w-48 animate-pulse rounded bg-muted" />
        <div className="h-40 animate-pulse rounded-lg bg-muted" />
      </div>
    );
  }

  if (!data) {
    return (
      <div className="text-center text-muted-foreground">用户不存在</div>
    );
  }

  const { user, hosts } = data;

  function handleToggleStatus() {
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

  function handleSetExpiry() {
    if (!expiryValue) return;
    updateExpiry.mutate(
      { userId: user.id, expiresAt: new Date(expiryValue).toISOString() },
      {
        onSuccess: () => {
          toast.success("到期时间已更新");
          setExpiryOpen(false);
        },
        onError: () => toast.error("操作失败"),
      },
    );
  }

  function handleClearExpiry() {
    updateExpiry.mutate(
      { userId: user.id, expiresAt: null },
      {
        onSuccess: () => {
          toast.success("已设为永不过期");
          setExpiryOpen(false);
        },
        onError: () => toast.error("操作失败"),
      },
    );
  }

  function openExpiryDialog() {
    if (user.expires_at) {
      const d = new Date(user.expires_at);
      const local = new Date(d.getTime() - d.getTimezoneOffset() * 60000)
        .toISOString()
        .slice(0, 16);
      setExpiryValue(local);
    } else {
      setExpiryValue("");
    }
    setExpiryOpen(true);
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" asChild>
          <Link to="/users">
            <ArrowLeft />
          </Link>
        </Button>
        <h1 className="text-2xl font-bold">{user.username}</h1>
        <Badge
          variant={
            user.status === "active"
              ? "default"
              : user.status === "expired"
                ? "destructive"
                : "secondary"
          }
        >
          {user.status === "active"
            ? "活跃"
            : user.status === "expired"
              ? "已过期"
              : "已禁用"}
        </Badge>
      </div>

      <div className="grid gap-6 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>用户信息</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="flex justify-between text-sm">
              <span className="text-muted-foreground">用户 ID</span>
              <span className="font-mono text-xs">{user.id}</span>
            </div>
            <Separator />
            <div className="flex items-start justify-between gap-2 text-sm">
              <span className="shrink-0 text-muted-foreground">短 ID</span>
              <div className="flex min-w-0 items-center gap-1">
                <span className="break-all font-mono text-xs">
                  {user.short_id ?? "—"}
                </span>
                {user.short_id ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 shrink-0"
                    onClick={() => {
                      navigator.clipboard.writeText(user.short_id!);
                      setShortIdCopied(true);
                      toast.success("已复制短 ID");
                      setTimeout(() => setShortIdCopied(false), 2000);
                    }}
                  >
                    {shortIdCopied ? (
                      <Check className="h-4 w-4 text-green-600" />
                    ) : (
                      <Copy className="h-4 w-4" />
                    )}
                  </Button>
                ) : null}
              </div>
            </div>
            <Separator />
            <div className="flex justify-between text-sm">
              <span className="text-muted-foreground">用户名</span>
              <span>{user.username}</span>
            </div>
            <Separator />
            <div className="flex justify-between text-sm">
              <span className="text-muted-foreground">创建时间</span>
              <span>{formatDate(user.created_at)}</span>
            </div>
            <Separator />
            <div className="flex justify-between text-sm">
              <span className="text-muted-foreground">更新时间</span>
              <span>{formatDate(user.updated_at)}</span>
            </div>
            <Separator />
            <div className="flex justify-between text-sm">
              <span className="text-muted-foreground">到期时间</span>
              <span>{user.expires_at ? formatDate(user.expires_at) : "永不过期"}</span>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>操作</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <Button
              variant="outline"
              className="w-full justify-start"
              onClick={handleToggleStatus}
              disabled={updateStatus.isPending}
            >
              {user.status === "active" ? (
                <>
                  <Ban className="mr-2 h-4 w-4" />
                  禁用用户
                </>
              ) : user.status === "expired" ? (
                <>
                  <CheckCircle className="mr-2 h-4 w-4" />
                  重新激活
                </>
              ) : (
                <>
                  <CheckCircle className="mr-2 h-4 w-4" />
                  启用用户
                </>
              )}
            </Button>
            <Button
              variant="outline"
              className="w-full justify-start"
              onClick={openExpiryDialog}
            >
              <Calendar className="mr-2 h-4 w-4" />
              设置到期时间
            </Button>
            <Button
              variant="outline"
              className="w-full justify-start"
              onClick={() => setRotateOpen(true)}
            >
              <KeyRound className="mr-2 h-4 w-4" />
              轮换登录密码
            </Button>
            <Button
              variant="destructive"
              className="w-full justify-start"
              onClick={() => setDeleteOpen(true)}
            >
              <Trash2 className="mr-2 h-4 w-4" />
              删除用户
            </Button>
          </CardContent>
        </Card>
      </div>

      <SSHKeyManager
        data={sshKeysQuery.data}
        isLoading={sshKeysQuery.isLoading}
        onGenerate={(keyType) =>
          generateSSHKey.mutate(
            { userId, keyType },
            {
              onSuccess: () => toast.success("SSH 密钥已生成"),
              onError: () => toast.error("生成失败"),
            },
          )
        }
        onSet={(publicKey, privateKey) =>
          setSSHKey.mutate(
            { userId, publicKey, privateKey },
            {
              onSuccess: () => toast.success("SSH 密钥已保存"),
              onError: () => toast.error("保存失败"),
            },
          )
        }
        onDelete={() =>
          deleteSSHKey.mutate(userId, {
            onSuccess: () => toast.success("SSH 密钥已删除"),
            onError: () => toast.error("删除失败"),
          })
        }
        isGenerating={generateSSHKey.isPending}
        isSetting={setSSHKey.isPending}
        isDeleting={deleteSSHKey.isPending}
      />

      <Card>
        <CardHeader>
          <CardTitle>主机列表</CardTitle>
        </CardHeader>
        <CardContent>
          {hosts.length === 0 ? (
            <p className="py-4 text-center text-sm text-muted-foreground">
              该用户暂无主机
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>主机 ID</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>Slot</TableHead>
                  <TableHead>创建时间</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {hosts.map((host) => (
                  <TableRow key={host.id}>
                    <TableCell className="font-mono text-xs">
                      <Link
                        to="/hosts/$hostId"
                        params={{ hostId: host.id }}
                        className="text-primary hover:underline"
                      >
                        {host.id}
                      </Link>
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={
                          host.status === "running" ? "default" : "secondary"
                        }
                      >
                        {host.status}
                      </Badge>
                    </TableCell>
                    <TableCell>{host.slot_key}</TableCell>
                    <TableCell className="text-muted-foreground">
                      {formatDate(host.created_at)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <RotatePasswordDialog
        userId={userId}
        open={rotateOpen}
        onOpenChange={setRotateOpen}
      />
      {deleteOpen && (
        <DeleteUserDialog
          user={user}
          open={deleteOpen}
          onOpenChange={(open) => {
            if (!open) setDeleteOpen(false);
          }}
          onDeleted={() => navigate({ to: "/users" })}
        />
      )}

      <Dialog open={expiryOpen} onOpenChange={setExpiryOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>设置到期时间</DialogTitle>
            <DialogDescription>
              设置用户的到期时间，到期后用户将自动进入过期状态。
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="expiry-input">到期时间</Label>
              <Input
                id="expiry-input"
                type="datetime-local"
                value={expiryValue}
                onChange={(e) => setExpiryValue(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={handleClearExpiry}
              disabled={updateExpiry.isPending}
            >
              清除到期时间
            </Button>
            <Button
              onClick={handleSetExpiry}
              disabled={!expiryValue || updateExpiry.isPending}
            >
              {updateExpiry.isPending ? "保存中…" : "确认"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
