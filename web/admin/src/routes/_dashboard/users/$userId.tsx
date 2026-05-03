import { useState } from "react";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import {
  Ban,
  Calendar,
  Check,
  CheckCircle,
  ChevronDown,
  Copy,
  KeyRound,
  RotateCcw,
  Server,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import {
  useUser,
  useUpdateUserStatus,
  useUpdateUserExpiry,
  useRegenerateCredentials,
  type RegenerateCredentialsResponse,
} from "@/hooks/use-users";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
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
  useAdminCreateSSHKey,
  useAdminDeleteSSHKey,
} from "@/hooks/use-ssh-keys";
import { EmptyState } from "@/components/layout/empty-state";

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
  const regenerateCreds = useRegenerateCredentials();
  const [rotateOpen, setRotateOpen] = useState(false);
  const [shortIdCopied, setShortIdCopied] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [expiryOpen, setExpiryOpen] = useState(false);
  const [expiryValue, setExpiryValue] = useState("");
  const [regenerateConfirmOpen, setRegenerateConfirmOpen] = useState(false);
  const [regeneratedCreds, setRegeneratedCreds] = useState<RegenerateCredentialsResponse | null>(null);
  const sshKeysQuery = useAdminSSHKeys(userId);
  const createSSHKey = useAdminCreateSSHKey();
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

  function handleRegenerateCreds() {
    regenerateCreds.mutate(userId, {
      onSuccess: (data) => {
        setRegenerateConfirmOpen(false);
        setRegeneratedCreds(data);
        toast.success("SSH 凭据已重新生成");
      },
      onError: () => toast.error("重新生成失败"),
    });
  }

  return (
    <div className="space-y-6">
      <nav aria-label="面包屑" className="text-sm text-muted-foreground">
        <Link to="/users" className="hover:text-foreground">
          用户管理
        </Link>
        <span className="mx-2 text-border">/</span>
        <span className="font-medium text-foreground">{user.username}</span>
      </nav>

      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-3">
            <h1 className="text-2xl font-bold tracking-tight">{user.username}</h1>
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
          <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <span className="font-mono">
              短 ID {user.short_id ?? "—"}
            </span>
            {user.short_id ? (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-7 gap-1 px-2"
                onClick={() => {
                  navigator.clipboard.writeText(user.short_id!);
                  setShortIdCopied(true);
                  toast.success("已复制短 ID");
                  setTimeout(() => setShortIdCopied(false), 2000);
                }}
              >
                {shortIdCopied ? (
                  <Check className="h-3.5 w-3.5 text-green-600" />
                ) : (
                  <Copy className="h-3.5 w-3.5" />
                )}
                复制
              </Button>
            ) : null}
          </div>
        </div>

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" className="gap-2">
              账户操作
              <ChevronDown className="h-4 w-4 opacity-60" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-52">
            <DropdownMenuItem
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
            </DropdownMenuItem>
            <DropdownMenuItem onClick={openExpiryDialog}>
              <Calendar className="mr-2 h-4 w-4" />
              设置到期时间
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => setRotateOpen(true)}>
              <KeyRound className="mr-2 h-4 w-4" />
              轮换登录密码
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => setRegenerateConfirmOpen(true)}>
              <RotateCcw className="mr-2 h-4 w-4" />
              重新生成 SSH 凭据
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              variant="destructive"
              onClick={() => setDeleteOpen(true)}
            >
              <Trash2 className="mr-2 h-4 w-4" />
              删除用户
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <Card className="overflow-hidden rounded-xl border-border/80 shadow-sm">
        <CardHeader className="border-b bg-muted/30">
          <CardTitle className="text-base">用户资料</CardTitle>
        </CardHeader>
        <CardContent className="p-6">
          <dl className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1">
              <dt className="text-xs font-medium text-muted-foreground">
                用户 ID
              </dt>
              <dd className="break-all font-mono text-sm">{user.id}</dd>
            </div>
            <div className="space-y-1">
              <dt className="text-xs font-medium text-muted-foreground">
                用户名
              </dt>
              <dd className="text-sm">{user.username}</dd>
            </div>
            <div className="space-y-1">
              <dt className="text-xs font-medium text-muted-foreground">
                创建时间
              </dt>
              <dd className="text-sm">{formatDate(user.created_at)}</dd>
            </div>
            <div className="space-y-1">
              <dt className="text-xs font-medium text-muted-foreground">
                更新时间
              </dt>
              <dd className="text-sm">{formatDate(user.updated_at)}</dd>
            </div>
            <div className="space-y-1 sm:col-span-2">
              <dt className="text-xs font-medium text-muted-foreground">
                到期时间
              </dt>
              <dd className="text-sm">
                {user.expires_at ? formatDate(user.expires_at) : "永不过期"}
              </dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      <SSHKeyManager
        keys={sshKeysQuery.data?.keys ?? []}
        isLoading={sshKeysQuery.isLoading}
        onCreate={(params) =>
          createSSHKey.mutate(
            { userId, ...params },
            {
              onSuccess: () => toast.success("SSH 密钥已创建"),
              onError: () => toast.error("创建失败"),
            },
          )
        }
        onDelete={(keyId) =>
          deleteSSHKey.mutate(
            { userId, keyId },
            {
              onSuccess: () => toast.success("SSH 密钥已删除"),
              onError: () => toast.error("删除失败"),
            },
          )
        }
        isCreating={createSSHKey.isPending}
        isDeleting={deleteSSHKey.isPending}
        lastCreatedKey={createSSHKey.data?.key}
      />

      <Card className="overflow-hidden rounded-xl border-border/80 shadow-sm">
        <CardHeader className="border-b bg-muted/30">
          <CardTitle className="text-base">主机列表</CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {hosts.length === 0 ? (
            <EmptyState
              icon={Server}
              title="该用户暂无主机"
              description="可在主机管理中为此用户新建云主机"
            />
          ) : (
            <div className="overflow-x-auto">
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
                            host.status === "running"
                              ? "default"
                              : host.status === "failed"
                                ? "destructive"
                                : host.status === "pending"
                                  ? "outline"
                                  : "secondary"
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
            </div>
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

      <Dialog
        open={regenerateConfirmOpen}
        onOpenChange={(open) => {
          if (!regenerateCreds.isPending) setRegenerateConfirmOpen(open);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>重新生成 SSH 凭据</DialogTitle>
            <DialogDescription>
              将覆盖该用户的 SSH 密码与 ed25519 入站密钥对（label=auto-generated 行将被替换）。
              生效后用户的旧 SSH 私钥与 sshpass 密码立即失效。请确认后再继续。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setRegenerateConfirmOpen(false)}
              disabled={regenerateCreds.isPending}
            >
              取消
            </Button>
            <Button
              type="button"
              variant="destructive"
              onClick={handleRegenerateCreds}
              disabled={regenerateCreds.isPending}
            >
              {regenerateCreds.isPending ? "生成中…" : "确认重新生成"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={!!regeneratedCreds}
        onOpenChange={(open) => {
          if (!open) setRegeneratedCreds(null);
        }}
      >
        <DialogContent className="sm:max-w-[560px]">
          <DialogHeader>
            <DialogTitle>SSH 凭据已生成</DialogTitle>
            <DialogDescription>
              请妥善保存以下凭据，关闭后无法再次查看。
            </DialogDescription>
          </DialogHeader>
          {regeneratedCreds && (
            <div className="max-h-[60vh] space-y-3 overflow-y-auto pr-1">
              <CredentialRow label="SSH 密码" value={regeneratedCreds.entry_password} />
              <CredentialBlock label="SSH 公钥" value={regeneratedCreds.ssh_public_key} />
              <CredentialBlock label="SSH 私钥（仅一次）" value={regeneratedCreds.ssh_private_key} />
              <CredentialRow label="SSH 公钥指纹" value={regeneratedCreds.ssh_key_fingerprint} />
            </div>
          )}
          <DialogFooter>
            <Button onClick={() => setRegeneratedCreds(null)}>关闭</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function CredentialRow({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false);
  function copy() {
    navigator.clipboard.writeText(value);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }
  return (
    <div className="space-y-1">
      <Label className="text-xs text-muted-foreground">{label}</Label>
      <div className="flex items-center gap-2">
        <code className="flex-1 truncate rounded bg-muted px-3 py-2 text-sm font-mono">
          {value}
        </code>
        <Button type="button" variant="ghost" size="icon" onClick={copy}>
          {copied ? (
            <Check className="h-4 w-4 text-green-500" />
          ) : (
            <Copy className="h-4 w-4" />
          )}
        </Button>
      </div>
    </div>
  );
}

function CredentialBlock({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false);
  function copy() {
    navigator.clipboard.writeText(value);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }
  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between">
        <Label className="text-xs text-muted-foreground">{label}</Label>
        <Button type="button" variant="ghost" size="sm" className="h-6 gap-1 px-2" onClick={copy}>
          {copied ? (
            <Check className="h-3.5 w-3.5 text-green-500" />
          ) : (
            <Copy className="h-3.5 w-3.5" />
          )}
          复制
        </Button>
      </div>
      <pre className="max-h-40 overflow-auto rounded bg-muted px-3 py-2 text-xs font-mono whitespace-pre-wrap break-all">
        {value}
      </pre>
    </div>
  );
}
