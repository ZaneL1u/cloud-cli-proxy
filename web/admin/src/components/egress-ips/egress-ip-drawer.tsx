import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import {
  useEgressIP,
  useCreateEgressIP,
  useUpdateEgressIP,
} from "@/hooks/use-egress-ips";
import {
  ProxyFields,
  formValuesToProxyConfig,
  proxyConfigToFormValues,
} from "./proxy-fields";
import { normalizeEgressIPAddress } from "@/lib/egress-ip-address";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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

const formSchema = z
  .object({
    label: z.string().min(1, "标签不能为空"),
    ip_address: z.string().default(""),
    provider: z.string().default("manual"),
    status: z.string().optional(),
    proxy_protocol: z
      .enum(["socks", "vmess", "vless", "shadowsocks", "trojan", "http"])
      .optional(),
    proxy_server: z.string().optional(),
    proxy_port: z.coerce.number().int().min(1).max(65535).optional(),
    proxy_username: z.string().optional(),
    proxy_password: z.string().optional(),
    proxy_uuid: z.string().optional(),
    proxy_security: z.string().optional(),
    proxy_alter_id: z.coerce.number().int().min(0).optional(),
    proxy_tls: z.boolean().optional(),
    proxy_server_name: z.string().optional(),
    proxy_method: z.string().optional(),
    proxy_transport_type: z.string().optional(),
    proxy_transport_path: z.string().optional(),
    proxy_transport_host: z.string().optional(),
    proxy_transport_service_name: z.string().optional(),
    proxy_tls_insecure: z.boolean().optional(),
    proxy_tls_alpn: z.string().optional(),
    proxy_flow: z.string().optional(),
    proxy_reality: z.boolean().optional(),
    proxy_reality_public_key: z.string().optional(),
    proxy_reality_short_id: z.string().optional(),
    edit_mode: z.enum(["form", "json"]).default("form"),
    proxy_config_json: z.string().optional(),
  })
  .superRefine((data, ctx) => {
    if (data.edit_mode === "form") {
      if (!data.proxy_server) {
        ctx.addIssue({
          code: "custom",
          path: ["proxy_server"],
          message: "服务器地址不能为空",
        });
      }
      if (!data.proxy_port) {
        ctx.addIssue({
          code: "custom",
          path: ["proxy_port"],
          message: "端口不能为空",
        });
      }
      const proto = data.proxy_protocol;
      if ((proto === "vmess" || proto === "vless") && !data.proxy_uuid) {
        ctx.addIssue({
          code: "custom",
          path: ["proxy_uuid"],
          message: "UUID 不能为空",
        });
      }
      if (proto === "shadowsocks") {
        if (!data.proxy_method)
          ctx.addIssue({
            code: "custom",
            path: ["proxy_method"],
            message: "加密方式不能为空",
          });
        if (!data.proxy_password)
          ctx.addIssue({
            code: "custom",
            path: ["proxy_password"],
            message: "密码不能为空",
          });
      }
      if (proto === "trojan" && !data.proxy_password) {
        ctx.addIssue({
          code: "custom",
          path: ["proxy_password"],
          message: "密码不能为空",
        });
      }
    }
    if (data.edit_mode === "json") {
      if (!data.proxy_config_json) {
        ctx.addIssue({
          code: "custom",
          path: ["proxy_config_json"],
          message: "JSON 配置不能为空",
        });
      } else {
        try {
          JSON.parse(data.proxy_config_json);
        } catch {
          ctx.addIssue({
            code: "custom",
            path: ["proxy_config_json"],
            message: "JSON 格式不正确",
          });
        }
      }
    }
  });

type FormValues = z.infer<typeof formSchema>;

const proxyDefaults = {
  proxy_protocol: "socks" as const,
  proxy_server: "",
  proxy_port: undefined as number | undefined,
  proxy_username: "",
  proxy_password: "",
  proxy_uuid: "",
  proxy_security: "auto",
  proxy_alter_id: 0,
  proxy_tls: false,
  proxy_server_name: "",
  proxy_method: "",
  proxy_transport_type: "",
  proxy_transport_path: "",
  proxy_transport_host: "",
  proxy_transport_service_name: "",
  proxy_tls_insecure: false,
  proxy_tls_alpn: "",
  proxy_flow: "",
  proxy_reality: false,
  proxy_reality_public_key: "",
  proxy_reality_short_id: "",
  edit_mode: "form" as const,
  proxy_config_json: "",
};

interface EgressIPDrawerProps {
  mode: "create" | "edit";
  egressIpId: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** 更新成功后回调（用于清除列表中缓存的检测结果等） */
  onUpdated?: (ipId: string) => void;
}

export function EgressIPDrawer({
  mode,
  egressIpId,
  open,
  onOpenChange,
  onUpdated,
}: EgressIPDrawerProps) {
  const { data: ipData } = useEgressIP(egressIpId ?? "");
  const createMutation = useCreateEgressIP();
  const updateMutation = useUpdateEgressIP();

  const form = useForm<FormValues>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      label: "",
      ip_address: "",
      provider: "manual",
      status: "available",
      ...proxyDefaults,
    },
  });

  useEffect(() => {
    if (mode === "edit" && ipData?.egress_ip) {
      const ip = ipData.egress_ip;
      const resetValues: any = {
        label: ip.label,
        ip_address: ip.ip_address,
        provider: ip.provider,
        status: ip.status,
        ...proxyDefaults,
      };

      if (ip.proxy_config) {
        const formVals = proxyConfigToFormValues(
          ip.proxy_config as Record<string, unknown>,
        );
        Object.assign(resetValues, formVals);
        if (resetValues.proxy_password === "***") {
          resetValues.proxy_password = "";
        }
        resetValues.proxy_config_json = JSON.stringify(
          ip.proxy_config,
          null,
          2,
        );
      }

      form.reset(resetValues);
    } else if (mode === "create") {
      form.reset({
        label: "",
        ip_address: "",
        provider: "manual",
        status: "available",
        ...proxyDefaults,
      });
    }
  }, [mode, ipData, form]);

  function onSubmit(values: FormValues) {
    // 空值表示尚未检测，后端验证阶段会通过 SOCKS5 探测真实出口 IP。
    const ipAddress = normalizeEgressIPAddress(values.ip_address);

    let proxyConfig: Record<string, unknown>;
    if (values.edit_mode === "json") {
      proxyConfig = JSON.parse(values.proxy_config_json!);
    } else {
      proxyConfig = formValuesToProxyConfig(values);
    }
    if (proxyConfig.password === "***" || proxyConfig.password === "") {
      delete proxyConfig.password;
    }

    const payload: Record<string, unknown> = {
      label: values.label,
      ip_address: ipAddress,
      provider: values.provider || "manual",
      proxy_config: proxyConfig,
    };

    if (mode === "edit") {
      payload.status = values.status;
    }

    if (mode === "create") {
      createMutation.mutate(payload as any, {
        onSuccess: () => {
          toast.success("出口 IP 已创建");
          onOpenChange(false);
        },
        onError: () => toast.error("创建失败"),
      });
    } else {
      updateMutation.mutate(
        { ipId: egressIpId!, data: payload as any },
        {
          onSuccess: () => {
            toast.success("出口 IP 已更新");
            if (egressIpId) onUpdated?.(egressIpId);
            onOpenChange(false);
          },
          onError: () => toast.error("更新失败"),
        },
      );
    }
  }

  const isPending = createMutation.isPending || updateMutation.isPending;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-[480px] overflow-y-auto sm:max-w-[480px]">
        <SheetHeader>
          <SheetTitle>
            {mode === "create" ? "添加出口 IP" : "编辑出口 IP"}
          </SheetTitle>
        </SheetHeader>

        <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4 p-4">
          <div className="space-y-2">
            <Label htmlFor="label">标签 *</Label>
            <Input id="label" {...form.register("label")} />
            {form.formState.errors.label && (
              <p className="text-sm text-destructive">
                {form.formState.errors.label.message}
              </p>
            )}
          </div>

          {mode === "edit" && form.watch("ip_address") && (
            <div className="space-y-1">
              <Label className="text-muted-foreground">出口 IP</Label>
              <p className="font-mono text-sm">{form.watch("ip_address")}</p>
            </div>
          )}

          {mode === "edit" && (
            <div className="space-y-2">
              <Label>状态</Label>
              <Select
                value={form.watch("status")}
                onValueChange={(val) => form.setValue("status", val)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="available">可用</SelectItem>
                  <SelectItem value="disabled">已禁用</SelectItem>
                </SelectContent>
              </Select>
            </div>
          )}

          <ProxyFields form={form} />

          <Button type="submit" className="w-full" disabled={isPending}>
            {isPending ? "保存中..." : mode === "create" ? "创建" : "保存"}
          </Button>
        </form>
      </SheetContent>
    </Sheet>
  );
}
