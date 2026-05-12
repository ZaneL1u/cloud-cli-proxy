import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Loader2 } from "lucide-react";
import {
  useCreateBypassRule,
  useUpdateBypassRule,
} from "@/hooks/use-bypass-rules";
import { parseBypassError } from "@/lib/i18n/bypass-error-codes";
import type {
  BypassRule,
  BypassRuleType,
} from "@/lib/api/types/bypass";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { RiskyKeywordConfirm } from "./risky-keyword-confirm";

// 5 种规则类型 placeholder（与 46-UI-SPEC 文案契约对齐）
const PLACEHOLDERS: Record<BypassRuleType, string> = {
  ip: "例如：192.168.1.10",
  cidr: "例如：10.0.0.0/8",
  domain: "例如：api.internal.corp",
  domain_suffix: "例如：corp.internal（不要带前导点）",
  domain_keyword: "例如：mirrors（≥ 4 字符）",
};

const TYPE_LABELS: Record<BypassRuleType, string> = {
  ip: "IP 地址",
  cidr: "CIDR 网段",
  domain: "完整域名",
  domain_suffix: "域名后缀",
  domain_keyword: "域名关键词",
};

// v3.5 不支持端口区分：plan 46-01 truth 没有 port 持久化承诺，
// 后端 BypassRule 也无 port 列。前端因此不再渲染端口输入。
const IPV4_RE =
  /^((25[0-5]|2[0-4]\d|[01]?\d?\d)\.){3}(25[0-5]|2[0-4]\d|[01]?\d?\d)$/;
const CIDR_RE = /^\S+\/\d{1,3}$/;
const DOMAIN_RE = /^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$/i;

const baseSchema = z
  .object({
    rule_type: z.enum([
      "ip",
      "cidr",
      "domain",
      "domain_suffix",
      "domain_keyword",
    ]),
    value: z.string().min(1, "值不能为空"),
    note: z.string().max(200, "备注不能超过 200 字").optional(),
  })
  .superRefine((data, ctx) => {
    switch (data.rule_type) {
      case "ip":
        if (!IPV4_RE.test(data.value)) {
          ctx.addIssue({
            code: "custom",
            path: ["value"],
            message: "IP 地址格式不合法",
          });
        }
        break;
      case "cidr":
        if (!CIDR_RE.test(data.value)) {
          ctx.addIssue({
            code: "custom",
            path: ["value"],
            message: "CIDR 格式不合法，例如 10.0.0.0/8",
          });
        }
        break;
      case "domain":
        if (!DOMAIN_RE.test(data.value)) {
          ctx.addIssue({
            code: "custom",
            path: ["value"],
            message: "请输入完整域名（如 api.internal.corp）",
          });
        }
        break;
      case "domain_suffix":
        if (data.value.startsWith(".")) {
          ctx.addIssue({
            code: "custom",
            path: ["value"],
            message: "不要带前导点",
          });
        }
        if (data.value.length < 4) {
          ctx.addIssue({
            code: "custom",
            path: ["value"],
            message: "域名后缀至少 4 字符",
          });
        }
        break;
      // domain_keyword 不在 schema 层硬拦截 < 4 字符 —— UX 走 RiskyKeywordConfirm 软确认
    }
  });

type FormValues = z.infer<typeof baseSchema>;

interface BypassRuleDrawerProps {
  hostId: string;
  mode: "create" | "edit";
  open: boolean;
  onOpenChange: (open: boolean) => void;
  rule?: BypassRule | null;
}

export function BypassRuleDrawer({
  hostId,
  mode,
  open,
  onOpenChange,
  rule,
}: BypassRuleDrawerProps) {
  const createMutation = useCreateBypassRule(hostId);
  const updateMutation = useUpdateBypassRule(hostId);

  const [riskyOpen, setRiskyOpen] = useState(false);
  const [pendingValues, setPendingValues] = useState<FormValues | null>(null);

  const form = useForm<FormValues>({
    resolver: zodResolver(baseSchema),
    defaultValues: {
      rule_type: "domain",
      value: "",
      note: "",
    },
  });

  useEffect(() => {
    if (!open) return;
    if (mode === "edit" && rule) {
      form.reset({
        rule_type: rule.rule_type,
        value: rule.value,
        note: rule.note ?? "",
      });
    } else {
      form.reset({
        rule_type: "domain",
        value: "",
        note: "",
      });
    }
  }, [open, mode, rule, form]);

  const ruleType = form.watch("rule_type");
  const value = form.watch("value");
  const keywordTooShort =
    ruleType === "domain_keyword" && value.length > 0 && value.length < 4;

  function submitPayload(values: FormValues, confirmRisky: boolean) {
    const payload = {
      rule_type: values.rule_type,
      value: values.value,
      note: values.note || undefined,
      confirm_risky: confirmRisky || undefined,
    };

    const onSuccess = () => {
      toast.success(mode === "create" ? "规则已创建" : "规则已更新");
      onOpenChange(false);
    };
    const onError = (err: unknown) => {
      const parsed = parseBypassError(err);
      toast.error(parsed.message);
    };

    if (mode === "create") {
      createMutation.mutate(payload, { onSuccess, onError });
    } else if (rule) {
      updateMutation.mutate(
        {
          ruleId: rule.id,
          payload: {
            value: payload.value,
            note: payload.note,
            confirm_risky: payload.confirm_risky,
          },
        },
        { onSuccess, onError },
      );
    }
  }

  function onSubmit(values: FormValues) {
    // domain_keyword < 4 → 弹软确认
    if (values.rule_type === "domain_keyword" && values.value.length < 4) {
      setPendingValues(values);
      setRiskyOpen(true);
      return;
    }
    submitPayload(values, false);
  }

  const isPending = createMutation.isPending || updateMutation.isPending;

  return (
    <>
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent className="w-[480px] overflow-y-auto sm:max-w-[480px]">
          <SheetHeader>
            <SheetTitle className="text-xl font-semibold tracking-tight">
              {mode === "create" ? "添加自定义规则" : "编辑自定义规则"}
            </SheetTitle>
          </SheetHeader>

          <form
            onSubmit={form.handleSubmit(onSubmit)}
            className="space-y-4 p-4"
            data-testid="bypass-rule-form"
          >
            <div className="space-y-2">
              <Label htmlFor="rule_type">规则类型 *</Label>
              <Select
                value={ruleType}
                onValueChange={(val) => {
                  form.setValue("rule_type", val as BypassRuleType);
                  // 切换类型时清空 value 字段，避免上次输入沿用到新校验
                  form.setValue("value", "");
                  form.clearErrors("value");
                }}
                disabled={mode === "edit"}
              >
                <SelectTrigger id="rule_type">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(
                    [
                      "ip",
                      "cidr",
                      "domain",
                      "domain_suffix",
                      "domain_keyword",
                    ] as BypassRuleType[]
                  ).map((t) => (
                    <SelectItem key={t} value={t}>
                      {TYPE_LABELS[t]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label htmlFor="value">值 *</Label>
              <Input
                id="value"
                placeholder={PLACEHOLDERS[ruleType]}
                {...form.register("value")}
                className="font-mono text-sm"
              />
              {keywordTooShort && (
                <p className="text-xs text-warning-foreground">
                  关键词较短，可能误命中其他域名
                </p>
              )}
              {form.formState.errors.value && (
                <p className="text-sm text-destructive">
                  {form.formState.errors.value.message}
                </p>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="note">备注</Label>
              <Textarea
                id="note"
                placeholder="简要说明此规则用途（≤ 200 字）"
                rows={3}
                {...form.register("note")}
              />
              {form.formState.errors.note && (
                <p className="text-sm text-destructive">
                  {form.formState.errors.note.message}
                </p>
              )}
            </div>

            <div className="flex justify-end gap-2 pt-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
                disabled={isPending}
              >
                取消
              </Button>
              <Button type="submit" disabled={isPending}>
                {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {isPending
                  ? "保存中…"
                  : mode === "create"
                    ? "创建规则"
                    : "保存修改"}
              </Button>
            </div>
          </form>
        </SheetContent>
      </Sheet>

      <RiskyKeywordConfirm
        open={riskyOpen}
        onOpenChange={setRiskyOpen}
        keyword={pendingValues?.value ?? ""}
        onConfirm={() => {
          if (pendingValues) submitPayload(pendingValues, true);
          setRiskyOpen(false);
        }}
      />
    </>
  );
}
