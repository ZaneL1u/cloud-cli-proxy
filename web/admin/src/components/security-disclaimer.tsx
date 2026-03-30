import { useState, useEffect, useRef } from "react";
import { ShieldAlert, ExternalLink } from "lucide-react";
import { Button } from "@/components/ui/button";

const DISCLAIMER_KEY = "security_disclaimer_accepted";
const DISCLAIMER_VERSION = "1";

function hasAccepted(): boolean {
  return localStorage.getItem(DISCLAIMER_KEY) === DISCLAIMER_VERSION;
}

function markAccepted(): void {
  localStorage.setItem(DISCLAIMER_KEY, DISCLAIMER_VERSION);
}

export function SecurityDisclaimer() {
  const [open, setOpen] = useState(false);
  const [canAccept, setCanAccept] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!hasAccepted()) {
      setOpen(true);
    }
  }, []);

  function handleScroll() {
    const el = scrollRef.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 30;
    if (atBottom) setCanAccept(true);
  }

  function handleAccept() {
    markAccepted();
    setOpen(false);
  }

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/70 backdrop-blur-sm p-4">
      <div className="w-full max-w-2xl rounded-xl bg-background shadow-2xl border">
        <div className="flex items-center gap-3 border-b px-6 py-4 bg-destructive/5">
          <ShieldAlert className="h-7 w-7 text-destructive shrink-0" />
          <div>
            <h2 className="text-lg font-bold text-destructive">安全与隐私声明</h2>
            <p className="text-sm text-muted-foreground">使用本平台前，请务必仔细阅读以下内容</p>
          </div>
        </div>

        <div
          ref={scrollRef}
          onScroll={handleScroll}
          className="max-h-[60vh] overflow-y-auto px-6 py-5 text-sm leading-relaxed space-y-5"
        >
          <section className="space-y-2">
            <h3 className="font-bold text-destructive">你的数据对管理员/宿主机所有者透明</h3>
            <ul className="list-disc pl-5 space-y-1.5 text-muted-foreground">
              <li>
                你的 <strong className="text-foreground">SSH 私钥</strong>在平台上以明文存储，
                平台管理员（即宿主机所有者）<strong className="text-foreground">可以直接查看和获取</strong>。
              </li>
              <li>
                管理员对你的容器拥有<strong className="text-foreground">完整的 root 权限</strong>，
                可以随时进入你的容器环境、读取任何文件、查看任何进程。
              </li>
              <li>
                你在容器中产生的所有网络流量都经由宿主机路由，理论上<strong className="text-foreground">可被宿主机截获</strong>。
              </li>
              <li>
                你的 <strong className="text-foreground">SSH 密码、登录密码</strong>同样由平台管理，管理员可重置和查看。
              </li>
            </ul>
          </section>

          <section className="space-y-2">
            <h3 className="font-bold text-green-700 dark:text-green-400">你应该这样做</h3>
            <ul className="list-disc pl-5 space-y-1.5 text-muted-foreground">
              <li>
                <strong className="text-foreground">为本平台生成专用 SSH 密钥</strong>，
                不要使用你在 GitHub、GitLab 或其他重要平台上的主力密钥。
              </li>
              <li>
                把本平台视为一个<strong className="text-foreground">公共开发环境</strong>，
                只放你愿意被他人看到的代码和数据。
              </li>
              <li>
                如果需要向 GitHub 等平台推送代码，使用<strong className="text-foreground">平台生成的专用密钥</strong>，
                并在对应平台上仅授予最小权限（如只读或单仓库访问）。
              </li>
            </ul>
          </section>

          <section className="space-y-2">
            <h3 className="font-bold text-destructive">千万不要这样做</h3>
            <ul className="list-disc pl-5 space-y-1.5 text-muted-foreground">
              <li>
                <strong className="text-foreground">千万不要</strong>在容器中存放生产环境的
                API Key、Token、数据库密码或任何关键凭证。
              </li>
              <li>
                <strong className="text-foreground">千万不要</strong>在容器中登录你的主要银行账户、
                重要邮箱或其他高敏感在线服务。
              </li>
              <li>
                <strong className="text-foreground">千万不要</strong>将你本地机器上的
                <code className="rounded bg-muted px-1">~/.ssh/id_rsa</code> 或
                <code className="rounded bg-muted px-1">~/.ssh/id_ed25519</code>
                复制到本平台 —— 这相当于把你的身份交给了平台管理员。
              </li>
              <li>
                <strong className="text-foreground">千万不要</strong>把包含敏感信息的
                <code className="rounded bg-muted px-1">.env</code> 文件放入容器。
              </li>
            </ul>
          </section>

          <section className="rounded-lg border border-amber-300 bg-amber-50 dark:border-amber-700 dark:bg-amber-950/30 p-4 space-y-2">
            <h3 className="font-bold text-amber-800 dark:text-amber-300">给新手的特别提醒</h3>
            <p className="text-amber-700 dark:text-amber-400">
              如果你不确定本平台运营者的身份和信誉，或者你还不完全理解上面提到的风险，
              <strong>请不要将自己的重要开发环境、私有代码仓库和个人凭证部署到本平台上</strong>。
              任何第三方托管的云主机系统，本质上就是在使用他人的服务器 ——
              你的数据安全完全依赖于对运营者的信任。
            </p>
          </section>

          <p className="text-xs text-muted-foreground pt-2">
            点击「我已阅读并知晓风险」即表示你已完整阅读上述内容，理解并接受相关风险。
          </p>
        </div>

        <div className="flex items-center justify-between border-t px-6 py-4 bg-muted/30">
          <p className="text-xs text-muted-foreground">
            请滚动至底部后方可确认
          </p>
          <Button
            size="lg"
            disabled={!canAccept}
            onClick={handleAccept}
            className="gap-2"
          >
            <ShieldAlert className="h-4 w-4" />
            我已阅读并知晓风险
          </Button>
        </div>
      </div>
    </div>
  );
}
