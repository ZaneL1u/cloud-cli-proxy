import { useState } from "react";
import { Copy, Check, Key, Download, Upload, Trash2, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import type { SSHKeyData } from "@/hooks/use-ssh-keys";

interface SSHKeyManagerProps {
  data: SSHKeyData | undefined;
  isLoading: boolean;
  onGenerate: (keyType: "ed25519" | "rsa") => void;
  onSet: (publicKey: string, privateKey: string) => void;
  onDelete?: () => void;
  isGenerating: boolean;
  isSetting: boolean;
  isDeleting?: boolean;
}

export function SSHKeyManager({
  data,
  isLoading,
  onGenerate,
  onSet,
  onDelete,
  isGenerating,
  isSetting,
  isDeleting,
}: SSHKeyManagerProps) {
  const [keyType, setKeyType] = useState<"ed25519" | "rsa">("ed25519");
  const [uploadOpen, setUploadOpen] = useState(false);
  const [uploadPub, setUploadPub] = useState("");
  const [uploadPriv, setUploadPriv] = useState("");
  const [pubCopied, setPubCopied] = useState(false);

  const hasKey = !!data?.public_key;

  function handleCopyPubKey() {
    if (!data?.public_key) return;
    navigator.clipboard.writeText(data.public_key).then(() => {
      setPubCopied(true);
      toast.success("公钥已复制到剪贴板");
      setTimeout(() => setPubCopied(false), 2000);
    });
  }

  function handleDownloadPrivKey() {
    if (!data?.private_key) return;
    const blob = new Blob([data.private_key], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    const filename =
      data.key_type === "rsa" ? "id_rsa" : "id_ed25519";
    a.href = url;
    a.download = filename;
    a.click();
    URL.revokeObjectURL(url);
    toast.success("私钥文件已下载");
  }

  function handleUploadSubmit() {
    const pub = uploadPub.trim();
    if (!pub) {
      toast.error("公钥不能为空");
      return;
    }
    onSet(pub, uploadPriv.trim());
    setUploadOpen(false);
    setUploadPub("");
    setUploadPriv("");
  }

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-lg">
            <Key className="h-5 w-5" />
            SSH 密钥
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="h-20 animate-pulse rounded bg-muted" />
        </CardContent>
      </Card>
    );
  }

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-lg">
            <Key className="h-5 w-5" />
            SSH 密钥
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {hasKey ? (
            <>
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium text-muted-foreground">
                    公钥（{data.key_type || "unknown"}）
                  </span>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-7 gap-1 text-xs"
                    onClick={handleCopyPubKey}
                  >
                    {pubCopied ? (
                      <Check className="h-3.5 w-3.5 text-green-600" />
                    ) : (
                      <Copy className="h-3.5 w-3.5" />
                    )}
                    {pubCopied ? "已复制" : "复制"}
                  </Button>
                </div>
                <pre className="max-h-24 overflow-auto whitespace-pre-wrap break-all rounded-lg border bg-muted/50 p-3 font-mono text-xs">
                  {data.public_key}
                </pre>
                <p className="text-xs text-muted-foreground">
                  可将此公钥添加到 GitHub、GitLab 等平台进行 Git 鉴权。
                </p>
              </div>

              {data.private_key && (
                <Button
                  variant="outline"
                  size="sm"
                  className="gap-1.5"
                  onClick={handleDownloadPrivKey}
                >
                  <Download className="h-3.5 w-3.5" />
                  下载私钥文件
                </Button>
              )}

              <div className="flex flex-wrap gap-2 pt-2">
                <AlertDialog>
                  <AlertDialogTrigger asChild>
                    <Button
                      variant="outline"
                      size="sm"
                      className="gap-1.5"
                      disabled={isGenerating}
                    >
                      <RefreshCw className="h-3.5 w-3.5" />
                      重新生成
                    </Button>
                  </AlertDialogTrigger>
                  <AlertDialogContent>
                    <AlertDialogHeader>
                      <AlertDialogTitle>重新生成 SSH 密钥？</AlertDialogTitle>
                      <AlertDialogDescription>
                        新密钥将覆盖现有密钥。如果已在 GitHub 等平台配置了旧公钥，需要重新上传新公钥。下次重建主机时新密钥会自动注入。
                      </AlertDialogDescription>
                    </AlertDialogHeader>
                    <div className="space-y-2">
                      <Label>密钥类型</Label>
                      <Select
                        value={keyType}
                        onValueChange={(v) =>
                          setKeyType(v as "ed25519" | "rsa")
                        }
                      >
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="ed25519">
                            Ed25519（推荐）
                          </SelectItem>
                          <SelectItem value="rsa">RSA 4096</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    <AlertDialogFooter>
                      <AlertDialogCancel>取消</AlertDialogCancel>
                      <AlertDialogAction
                        disabled={isGenerating}
                        onClick={() => onGenerate(keyType)}
                      >
                        {isGenerating ? "生成中..." : "确认生成"}
                      </AlertDialogAction>
                    </AlertDialogFooter>
                  </AlertDialogContent>
                </AlertDialog>

                <Button
                  variant="outline"
                  size="sm"
                  className="gap-1.5"
                  onClick={() => setUploadOpen(true)}
                >
                  <Upload className="h-3.5 w-3.5" />
                  导入已有密钥
                </Button>

                {onDelete && (
                  <AlertDialog>
                    <AlertDialogTrigger asChild>
                      <Button
                        variant="outline"
                        size="sm"
                        className="gap-1.5 text-destructive hover:text-destructive"
                        disabled={isDeleting}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                        删除密钥
                      </Button>
                    </AlertDialogTrigger>
                    <AlertDialogContent>
                      <AlertDialogHeader>
                        <AlertDialogTitle>确认删除 SSH 密钥？</AlertDialogTitle>
                        <AlertDialogDescription>
                          删除后需要重新生成或导入密钥。已在 GitHub 等平台配置的旧公钥也需清理。
                        </AlertDialogDescription>
                      </AlertDialogHeader>
                      <AlertDialogFooter>
                        <AlertDialogCancel>取消</AlertDialogCancel>
                        <AlertDialogAction
                          disabled={isDeleting}
                          onClick={onDelete}
                        >
                          {isDeleting ? "删除中..." : "确认删除"}
                        </AlertDialogAction>
                      </AlertDialogFooter>
                    </AlertDialogContent>
                  </AlertDialog>
                )}
              </div>
            </>
          ) : (
            <div className="space-y-4">
              <p className="text-sm text-muted-foreground">
                尚未配置 SSH 密钥。生成后可用于 GitHub、GitLab 等平台的 Git
                鉴权，并会在主机新建/重建时自动注入。
              </p>
              <div className="flex flex-wrap items-end gap-3">
                <div className="space-y-1.5">
                  <Label className="text-xs">密钥类型</Label>
                  <Select
                    value={keyType}
                    onValueChange={(v) =>
                      setKeyType(v as "ed25519" | "rsa")
                    }
                  >
                    <SelectTrigger className="w-44">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="ed25519">
                        Ed25519（推荐）
                      </SelectItem>
                      <SelectItem value="rsa">RSA 4096</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <Button
                  size="sm"
                  className="gap-1.5"
                  disabled={isGenerating}
                  onClick={() => onGenerate(keyType)}
                >
                  <Key className="h-3.5 w-3.5" />
                  {isGenerating ? "生成中..." : "生成密钥"}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="gap-1.5"
                  onClick={() => setUploadOpen(true)}
                >
                  <Upload className="h-3.5 w-3.5" />
                  导入已有密钥
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      <Dialog open={uploadOpen} onOpenChange={setUploadOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>导入 SSH 密钥</DialogTitle>
            <DialogDescription>
              粘贴你已有的 SSH 公钥和私钥。公钥为必填项，私钥可选（不填则仅保存公钥）。
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="upload-pub">公钥（必填）</Label>
              <textarea
                id="upload-pub"
                className="w-full rounded-md border bg-muted/50 p-2 font-mono text-xs"
                rows={3}
                placeholder="ssh-ed25519 AAAA... 或 ssh-rsa AAAA..."
                value={uploadPub}
                onChange={(e) => setUploadPub(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="upload-priv">私钥（可选）</Label>
              <textarea
                id="upload-priv"
                className="w-full rounded-md border bg-muted/50 p-2 font-mono text-xs"
                rows={5}
                placeholder="-----BEGIN OPENSSH PRIVATE KEY-----&#10;..."
                value={uploadPriv}
                onChange={(e) => setUploadPriv(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setUploadOpen(false)}
            >
              取消
            </Button>
            <Button
              onClick={handleUploadSubmit}
              disabled={!uploadPub.trim() || isSetting}
            >
              {isSetting ? "保存中..." : "保存"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
