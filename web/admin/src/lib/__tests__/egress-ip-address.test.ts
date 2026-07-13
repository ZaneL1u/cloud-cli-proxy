import { describe, expect, it } from "vitest";
import { normalizeEgressIPAddress } from "../egress-ip-address";

describe("normalizeEgressIPAddress", () => {
  it("keeps an undetected address empty", () => {
    expect(normalizeEgressIPAddress("   ")).toBe("");
  });

  it("trims an explicitly supplied address", () => {
    expect(normalizeEgressIPAddress(" 203.0.113.10 ")).toBe("203.0.113.10");
  });
});
