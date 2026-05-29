import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { CustomRulesTable } from "../custom-rules-table";
import type { BypassRule } from "@/lib/api/types/bypass";

const apiFetchMock = vi.fn();
vi.mock("@/lib/api", () => ({
  apiFetch: (...args: unknown[]) => apiFetchMock(...args),
}));
vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

function renderWithClient(ui: React.ReactNode) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

const rules: BypassRule[] = [
  {
    id: "r-1",
    host_id: "h-1",
    rule_type: "cidr",
    value: "10.0.0.0/24",
    is_risky: false,
    note: "内网网段",
    created_at: "",
    updated_at: "",
  },
  {
    id: "r-2",
    host_id: "h-1",
    rule_type: "domain_keyword",
    value: "abc",
    is_risky: true,
    note: "短关键词",
    created_at: "",
    updated_at: "",
  },
];

describe("CustomRulesTable", () => {
  beforeEach(() => {
    apiFetchMock.mockReset();
  });

  it("空状态展示「暂无规则」+ 添加按钮", async () => {
    apiFetchMock.mockResolvedValue({ rules: [] });
    renderWithClient(<CustomRulesTable hostId="h-1" />);

    expect(
      await screen.findByText("暂无规则"),
    ).toBeInTheDocument();
    const addButtons = screen.getAllByText("添加规则");
    expect(addButtons.length).toBeGreaterThanOrEqual(1);
  });

  it("点击删除按钮弹出二次确认 AlertDialog", async () => {
    apiFetchMock.mockResolvedValue({ rules });
    const user = userEvent.setup();
    renderWithClient(<CustomRulesTable hostId="h-1" />);

    const row1 = await screen.findByTestId("rules-row-r-1");
    const deleteBtn = row1.querySelector(
      'button[aria-label="删除规则"]',
    ) as HTMLButtonElement;
    await user.click(deleteBtn);

    expect(await screen.findByText("删除该规则？")).toBeInTheDocument();
    expect(
      screen.getByText(/删除后需点击/i),
    ).toBeInTheDocument();
  });

  it("搜索框过滤规则", async () => {
    apiFetchMock.mockResolvedValue({ rules });
    const user = userEvent.setup();
    renderWithClient(<CustomRulesTable hostId="h-1" />);

    await screen.findByTestId("rules-row-r-1");

    const search = screen.getByPlaceholderText("搜索值或备注…");
    await user.type(search, "abc");

    // r-1 (cidr 10.0.0.0/24, note 内网网段) 不匹配 "abc"
    expect(screen.queryByTestId("rules-row-r-1")).not.toBeInTheDocument();
    // r-2 (keyword abc) 匹配
    expect(screen.getByTestId("rules-row-r-2")).toBeInTheDocument();
  });
});
