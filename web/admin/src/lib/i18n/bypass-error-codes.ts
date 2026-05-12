// BYPASS_* 错误码 → 中文文案。
// 与 46-UI-SPEC.md「错误状态」表保持一致；46-CONTEXT.md 锁定本文件提供 i18n 兜底。
//
// handler 返回 `{"code":"BYPASS_*","message":"..."}` 结构；前端 toast / Drawer / Sheet 共用此映射。

export const BYPASS_ERROR_MESSAGES: Record<string, string> = {
  BYPASS_RULE_TOO_BROAD:
    "规则覆盖范围过大，请使用更具体的 CIDR 或域名",
  BYPASS_RULE_CONFLICT_PROXY:
    "该规则覆盖了代理服务器 IP，会导致代理自身无法访问",
  BYPASS_LIMIT_EXCEEDED:
    "当前 host 自定义规则已达上限（1000 条），请先删除不再使用的规则",
  BYPASS_KEYWORD_TOO_SHORT:
    "关键词长度小于 4 字符，存在误命中风险，请勾选「我已知悉」继续",
  BYPASS_PRESET_IMMUTABLE: "系统预设不可修改或删除",
  BYPASS_HOST_UNREACHABLE:
    "host-agent 当前不可达，配置已保存但未生效，将自动重试",
  BYPASS_RELOAD_TIMEOUT: "sing-box reload 超时，已自动回滚",
  BYPASS_VALIDATION_FAILED: "规则校验失败，请检查输入",
};

export function bypassErrorMessage(code: string | undefined | null): string {
  if (!code) return "操作失败，请稍后重试";
  const msg = BYPASS_ERROR_MESSAGES[code];
  if (msg) return msg;
  return `操作失败，请稍后重试 · 错误码：${code}`;
}

// 尝试从 apiFetch 抛出的 ApiError.message（HTTP body 文本）里解析错误码。
// handler 约定的 body 是 `{"code":"BYPASS_*","message":"..."}`，但也可能是纯文本错误。
export function parseBypassError(raw: unknown): {
  code: string | null;
  message: string;
} {
  if (typeof raw !== "object" || raw === null) {
    return { code: null, message: bypassErrorMessage(null) };
  }
  const err = raw as { message?: string };
  if (!err.message) {
    return { code: null, message: bypassErrorMessage(null) };
  }
  try {
    const parsed = JSON.parse(err.message);
    if (
      parsed &&
      typeof parsed === "object" &&
      "code" in parsed &&
      typeof parsed.code === "string"
    ) {
      return {
        code: parsed.code,
        message: bypassErrorMessage(parsed.code),
      };
    }
  } catch {
    // 非 JSON body，退回兜底文案
  }
  return { code: null, message: bypassErrorMessage(null) };
}
