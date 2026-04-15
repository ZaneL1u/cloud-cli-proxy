import { useState, useEffect, useCallback } from "react";
import {
  UserPlus,
  Server,
  Globe,
  Play,
  Key,
  Terminal,
  Monitor,
  Lock,
  ShieldCheck,
  Rocket,
  ChevronLeft,
  ChevronRight,
  X,
  type LucideIcon,
} from "lucide-react";
import { Button } from "@/components/ui/button";

const ONBOARDING_KEY_PREFIX = "onboarding_completed_";
const ONBOARDING_VERSION = "1";

interface Step {
  icon: LucideIcon;
  title: string;
  description: string;
  details: string[];
  tip?: string;
}

const adminSteps: Step[] = [
  {
    icon: UserPlus,
    title: "创建用户",
    description: "在「用户管理」页面为团队成员创建账号",
    details: [
      "点击左侧菜单「用户管理」",
      "点击「新建用户」按钮，输入用户名",
      "系统会自动生成登录密码与用户短 ID",
      "将用户名与登录密码发给用户（仅此一次可见）；短 ID 用于 curl 一键连接等场景",
    ],
    tip: "Web 后台与门户使用「用户名 + 登录密码」登录；短 ID 不是网页登录账号，但需告知用户以便使用一键连接。",
  },
  {
    icon: Globe,
    title: "添加出口 IP",
    description: "在「出口 IP」页面配置代理隧道",
    details: [
      "点击左侧菜单「出口 IP」",
      "点击「添加」按钮，填写 IP 地址和隧道配置",
      "支持多种代理协议（SOCKS、VMess、VLESS、Shadowsocks、Trojan、HTTP）",
      "确认 IP 状态为「可用」",
    ],
    tip: "没有出口 IP 的主机无法启动，请至少配置一个。",
  },
  {
    icon: Server,
    title: "创建主机",
    description: "在「主机管理」页面为用户分配云主机",
    details: [
      "点击左侧菜单「主机管理」",
      "点击「创建主机」，选择要分配的用户",
      "配置主机名、时区、资源限制等参数",
      "系统会自动生成 SSH 短 ID 和密码",
    ],
  },
  {
    icon: Globe,
    title: "绑定出口 IP",
    description: "为主机绑定出口 IP 以启用网络",
    details: [
      "进入主机详情页",
      "在「出口 IP 绑定」区域点击「绑定」",
      "选择一个可用的出口 IP",
      "绑定后主机的所有流量都将通过该 IP 出网",
    ],
    tip: "必须绑定出口 IP 后才能启动主机。",
  },
  {
    icon: Play,
    title: "启动主机",
    description: "启动主机，等待容器就绪",
    details: [
      "在主机详情页点击「启动」按钮",
      "系统会创建容器、配置网络隧道、启动 SSH",
      "可在「任务列表」页面查看启动进度",
      "状态变为「运行中」即可使用",
    ],
  },
  {
    icon: Key,
    title: "管理 SSH 密钥",
    description: "为用户管理入站和出站 SSH 密钥",
    details: [
      "进入用户详情页，找到「SSH 密钥管理」区域",
      "入站密钥：添加用户本地公钥，实现免密 SSH 登录",
      "出站密钥：生成密钥对，公钥添加到 GitHub 等平台用于 Git 鉴权",
      "密钥会在主机启动/重建时自动注入容器",
    ],
    tip: "用户也可以在自己的门户页面管理密钥。",
  },
  {
    icon: ShieldCheck,
    title: "就绪！",
    description: "告知用户连接信息即可开始使用",
    details: [
      "将主机的 SSH 短 ID 和密码告知用户",
      "用户可通过 curl 一键登录或 SSH 直连",
      "管理后台可实时查看任务和事件日志",
      "定期检查主机状态和到期时间",
    ],
  },
];

const userSteps: Step[] = [
  {
    icon: Rocket,
    title: "欢迎使用",
    description: "这是你的专属云开发环境",
    details: [
      "管理员已为你创建了账号和云主机",
      "你可以通过 SSH 或浏览器桌面访问主机",
      "所有网络流量都通过指定出口 IP 路由",
      "home 目录（/workspace）的数据在重建后保留",
    ],
  },
  {
    icon: Terminal,
    title: "SSH 连接",
    description: "使用 SSH 连接到你的云主机",
    details: [
      "进入「我的主机」→ 点击主机卡片查看详情",
      "复制 curl 命令在终端粘贴执行，一键连接",
      "或者复制 SSH 命令手动连接（需输入 SSH 密码）",
      "SSH 密码由管理员管理，与登录密码不同",
    ],
    tip: "curl 命令输入登录密码后会自动完成 SSH 连接。",
  },
  {
    icon: Monitor,
    title: "桌面访问",
    description: "通过浏览器直接使用图形桌面",
    details: [
      "在主机详情页点击「打开桌面（VNC）」",
      "无需安装任何客户端，浏览器内直接操作",
      "桌面预装了 Chromium 浏览器",
      "支持剪贴板同步和分辨率自适应",
    ],
  },
  {
    icon: Key,
    title: "SSH 密钥管理",
    description: "管理入站和出站 SSH 密钥",
    details: [
      "在主机详情页找到「SSH 密钥管理」区域",
      "入站密钥：添加你本地电脑的公钥，免密 SSH 连接到云主机",
      "出站密钥：生成密钥对，将公钥添加到 GitHub / GitLab 用于 Git 鉴权",
      "密钥在主机重建后会自动重新注入",
    ],
    tip: "建议为本平台生成专用的出站密钥，不要使用你本地的主力密钥。",
  },
  {
    icon: Lock,
    title: "修改登录密码",
    description: "你可以随时修改你的登录密码",
    details: [
      "在「我的面板」首页找到「修改登录密码」卡片",
      "输入当前密码和新密码即可更新",
      "登录密码用于网页登录和 curl 入口",
      "SSH 密码由管理员管理，如需修改请联系管理员",
    ],
  },
  {
    icon: ShieldCheck,
    title: "准备就绪！",
    description: "开始享受你的云开发环境吧",
    details: [
      "在容器内可以正常使用 git、npm、python 等工具",
      "预装了 claude code，可直接使用 AI 编程助手",
      "如遇问题可联系管理员获取帮助",
      "重建主机不会丢失 /workspace 下的数据",
    ],
  },
];

interface OnboardingGuideProps {
  role: "admin" | "user";
  forceOpen?: boolean;
  onClose?: () => void;
}

export function OnboardingGuide({
  role,
  forceOpen,
  onClose,
}: OnboardingGuideProps) {
  const storageKey = ONBOARDING_KEY_PREFIX + role;
  const [open, setOpen] = useState(false);
  const [step, setStep] = useState(0);

  const steps = role === "admin" ? adminSteps : userSteps;

  useEffect(() => {
    if (forceOpen) {
      setStep(0);
      setOpen(true);
      return;
    }
    if (localStorage.getItem(storageKey) !== ONBOARDING_VERSION) {
      setOpen(true);
    }
  }, [forceOpen, storageKey]);

  const handleClose = useCallback(() => {
    localStorage.setItem(storageKey, ONBOARDING_VERSION);
    setOpen(false);
    setStep(0);
    onClose?.();
  }, [storageKey, onClose]);

  useEffect(() => {
    if (!open) return;
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") {
        handleClose();
      } else if (e.key === "ArrowRight" || e.key === "Enter") {
        if (step < steps.length - 1) setStep((s) => s + 1);
        else handleClose();
      } else if (e.key === "ArrowLeft") {
        if (step > 0) setStep((s) => s - 1);
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [open, step, steps.length, handleClose]);

  if (!open) return null;

  const current = steps[step];
  const isLast = step === steps.length - 1;
  const Icon = current.icon;

  return (
    <div
      className="fixed inset-0 z-9998 flex items-center justify-center bg-black/50 backdrop-blur-sm p-4"
      onClick={(e) => {
        if (e.target === e.currentTarget) handleClose();
      }}
    >
      <div className="w-full max-w-lg rounded-2xl bg-background shadow-2xl border overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between px-6 pt-5 pb-0">
          <p className="text-xs font-medium text-muted-foreground tracking-wide">
            {role === "admin" ? "管理员引导" : "使用引导"} · {step + 1} / {steps.length}
          </p>
          <button
            onClick={handleClose}
            className="flex h-7 w-7 items-center justify-center rounded-lg text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Progress */}
        <div className="px-6 pt-3">
          <div className="flex gap-1">
            {steps.map((_, i) => (
              <button
                key={i}
                onClick={() => setStep(i)}
                className="h-1 flex-1 rounded-full transition-all duration-300"
                style={{
                  backgroundColor:
                    i <= step
                      ? "hsl(var(--primary))"
                      : "hsl(var(--muted))",
                  opacity: i <= step ? 1 : 0.6,
                }}
              />
            ))}
          </div>
        </div>

        {/* Content */}
        <div className="px-6 pt-6 pb-2 space-y-4">
          <div className="flex items-start gap-4">
            <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-primary/10">
              <Icon className="h-6 w-6 text-primary" />
            </div>
            <div className="space-y-1 pt-0.5">
              <h3 className="text-lg font-semibold leading-tight">{current.title}</h3>
              <p className="text-sm text-muted-foreground">{current.description}</p>
            </div>
          </div>

          <div className="space-y-2 pl-1">
            {current.details.map((detail, i) => (
              <div key={i} className="flex items-start gap-3 text-sm">
                <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-md bg-muted text-[11px] font-semibold text-muted-foreground mt-0.5">
                  {i + 1}
                </span>
                <span className="text-foreground/80 leading-relaxed">{detail}</span>
              </div>
            ))}
          </div>

          {current.tip && (
            <p className="text-xs text-muted-foreground border-l-2 border-primary/30 pl-3 ml-1">
              {current.tip}
            </p>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between px-6 py-4 mt-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setStep((s) => s - 1)}
            disabled={step === 0}
            className="gap-1"
          >
            <ChevronLeft className="h-4 w-4" />
            上一步
          </Button>
          <Button
            size="sm"
            onClick={() => {
              if (isLast) handleClose();
              else setStep((s) => s + 1);
            }}
            className="gap-1"
          >
            {isLast ? "开始使用" : "下一步"}
            {!isLast && <ChevronRight className="h-4 w-4" />}
          </Button>
        </div>
      </div>
    </div>
  );
}

export function useOnboardingGuide() {
  const [forceOpen, setForceOpen] = useState(false);
  return {
    forceOpen,
    openGuide: () => setForceOpen(true),
    onClose: () => setForceOpen(false),
  };
}
