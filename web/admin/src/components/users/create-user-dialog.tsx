import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Copy, Check } from "lucide-react";
import { useCreateUser } from "@/hooks/use-users";
import { ApiError } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";

const schema = z.object({
  username: z
    .string()
    .min(3, "用户名至少 3 个字符")
    .max(50, "用户名最多 50 个字符"),
});

type FormValues = z.infer<typeof schema>;

interface Credentials {
  username: string;
  password: string;
  short_id: string;
  entry_password: string;
  ssh_public_key: string;
  ssh_private_key: string;
  ssh_key_fingerprint: string;
}

interface CreateUserDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

function CopyField({ label, value }: { label: string; value: string }) {
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

function CopyTextarea({ label, value }: { label: string; value: string }) {
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

export function CreateUserDialog({
  open,
  onOpenChange,
}: CreateUserDialogProps) {
  const createUser = useCreateUser();
  const [credentials, setCredentials] = useState<Credentials | null>(null);
  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
  });

  function handleClose() {
    reset();
    setCredentials(null);
    onOpenChange(false);
  }

  function onSubmit(data: FormValues) {
    createUser.mutate(data, {
      onSuccess: (res) => {
        toast.success("用户创建成功");
        setCredentials({
          username: res.user.username,
          password: res.password,
          short_id: res.short_id,
          entry_password: res.entry_password,
          ssh_public_key: res.ssh_public_key,
          ssh_private_key: res.ssh_private_key,
          ssh_key_fingerprint: res.ssh_key_fingerprint,
        });
      },
      onError: (err) => {
        if (err instanceof ApiError && err.status === 409) {
          toast.error("用户名已存在");
        } else {
          toast.error("创建失败");
        }
      },
    });
  }

  if (credentials) {
    return (
      <Dialog open={open} onOpenChange={handleClose}>
        <DialogContent className="sm:max-w-[560px]">
          <DialogHeader>
            <DialogTitle>用户创建成功</DialogTitle>
            <DialogDescription>
              请妥善保存以下凭据，登录密码、SSH 密码与 SSH 私钥仅在此次显示。
            </DialogDescription>
          </DialogHeader>
          <div className="max-h-[60vh] space-y-3 overflow-y-auto pr-1">
            <CopyField label="用户名（网页登录 / SSH 用户名）" value={credentials.username} />
            <CopyField label="登录密码（网页登录）" value={credentials.password} />
            <CopyField label="SSH 密码（curl 一键连接 / sshpass）" value={credentials.entry_password} />
            <CopyTextarea label="SSH 公钥（用于 known_hosts / 备份）" value={credentials.ssh_public_key} />
            <CopyTextarea label="SSH 私钥（用户分发，仅一次）" value={credentials.ssh_private_key} />
            <CopyField label="SSH 公钥指纹" value={credentials.ssh_key_fingerprint} />
            <CopyField label="用户短 ID" value={credentials.short_id} />
            <p className="text-xs text-muted-foreground">
              用户使用「用户名 + 登录密码」登录管理后台/门户。SSH 凭据用于 curl 一键连接和 sshpass 等场景，丢失后只能通过「重新生成 SSH 凭据」覆盖。
            </p>
          </div>
          <DialogFooter>
            <Button onClick={handleClose}>关闭</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    );
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>创建用户</DialogTitle>
          <DialogDescription>
            创建用户后系统会一次性生成登录密码、SSH 密码与 ed25519 SSH 密钥对，关闭弹窗后无法再次查看。
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="username">用户名</Label>
            <Input
              id="username"
              placeholder="输入用户名"
              {...register("username")}
            />
            {errors.username && (
              <p className="text-sm text-destructive">
                {errors.username.message}
              </p>
            )}
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              取消
            </Button>
            <Button type="submit" disabled={createUser.isPending}>
              {createUser.isPending ? "创建中…" : "创建"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
