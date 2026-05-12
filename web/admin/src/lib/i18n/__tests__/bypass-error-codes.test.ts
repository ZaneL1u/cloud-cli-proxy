import { describe, it, expect } from "vitest";
import {
  bypassErrorMessage,
  parseBypassError,
  BYPASS_ERROR_MESSAGES,
} from "../bypass-error-codes";

describe("bypassErrorMessage", () => {
  it("已知错误码返回对应中文文案", () => {
    expect(bypassErrorMessage("BYPASS_RULE_TOO_BROAD")).toContain(
      "覆盖范围过大",
    );
    expect(bypassErrorMessage("BYPASS_LIMIT_EXCEEDED")).toContain("1000");
    expect(bypassErrorMessage("BYPASS_PRESET_IMMUTABLE")).toContain(
      "系统预设",
    );
  });

  it("未知错误码使用兜底文案并附错误码", () => {
    expect(bypassErrorMessage("UNKNOWN_X")).toContain("UNKNOWN_X");
  });

  it("null / undefined / 空字符串返回纯兜底", () => {
    expect(bypassErrorMessage(null)).toBe("操作失败，请稍后重试");
    expect(bypassErrorMessage(undefined)).toBe("操作失败，请稍后重试");
    expect(bypassErrorMessage("")).toBe("操作失败，请稍后重试");
  });
});

describe("parseBypassError", () => {
  it("从 JSON body 中解析 code 字段", () => {
    const err = {
      message: JSON.stringify({
        code: "BYPASS_RULE_TOO_BROAD",
        message: "anything",
      }),
    };
    const out = parseBypassError(err);
    expect(out.code).toBe("BYPASS_RULE_TOO_BROAD");
    expect(out.message).toBe(BYPASS_ERROR_MESSAGES.BYPASS_RULE_TOO_BROAD);
  });

  it("非 JSON body 退回兜底，无错误码", () => {
    const err = { message: "plain text 500" };
    const out = parseBypassError(err);
    expect(out.code).toBeNull();
    expect(out.message).toBe("操作失败，请稍后重试");
  });

  it("非对象输入也安全", () => {
    expect(parseBypassError(null).code).toBeNull();
    expect(parseBypassError(undefined).code).toBeNull();
    expect(parseBypassError("oops").code).toBeNull();
  });
});
