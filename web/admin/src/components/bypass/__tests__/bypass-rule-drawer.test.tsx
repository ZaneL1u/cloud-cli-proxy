import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BypassRuleDrawer } from "../bypass-rule-drawer";

// 拦截 apiFetch；mock 必须在文件顶层并使用工厂函数（vitest hoisting 规则）
vi.mock("@/lib/api", () => ({
  apiFetch: vi.fn(async () => ({ rule: { id: "new-rule-id" } })),
}));

// sonner toast 不在 jsdom 环境中工作，stub 掉
vi.mock("sonner", () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
  },
}));

function renderWithClient(ui: React.ReactNode) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe("BypassRuleDrawer", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("create 模式默认渲染规则类型选择 + 值字段", async () => {
    renderWithClient(
      <BypassRuleDrawer
        hostId="h-1"
        mode="create"
        open={true}
        onOpenChange={() => {}}
      />,
    );

    expect(
      await screen.findByText("添加自定义规则"),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("规则类型 *")).toBeInTheDocument();
    expect(screen.getByLabelText("值 *")).toBeInTheDocument();
    expect(screen.getByText("创建规则")).toBeInTheDocument();
  });

  it("edit 模式锁定规则类型选择器（disabled）", async () => {
    renderWithClient(
      <BypassRuleDrawer
        hostId="h-1"
        mode="edit"
        open={true}
        onOpenChange={() => {}}
        rule={{
          id: "r-1",
          host_id: "h-1",
          rule_type: "ip",
          value: "10.0.0.1",
          is_risky: false,
          note: "test",
          created_at: "",
          updated_at: "",
        }}
      />,
    );

    expect(
      await screen.findByText("编辑自定义规则"),
    ).toBeInTheDocument();
    const trigger = screen.getByLabelText("规则类型 *");
    expect(trigger).toHaveAttribute("data-disabled");
    expect(screen.getByText("保存修改")).toBeInTheDocument();
  });

  it("zod 校验：默认 domain 类型输入非法格式时显示错误", async () => {
    const user = userEvent.setup();
    renderWithClient(
      <BypassRuleDrawer
        hostId="h-1"
        mode="create"
        open={true}
        onOpenChange={() => {}}
      />,
    );

    // 默认 rule_type=domain，直接填入非法值触发校验
    const valueInput = await screen.findByLabelText("值 *");
    await user.type(valueInput, "not-a-domain");
    await user.click(screen.getByText("创建规则"));

    expect(
      await screen.findByText(/请输入完整域名/i),
    ).toBeInTheDocument();
  });

  it("domain_keyword 类型 < 4 字符显示 inline warning", async () => {
    const user = userEvent.setup();
    // 直接构造 edit 模式 + domain_keyword 类型规则（type 锁定），避免 Select 交互
    renderWithClient(
      <BypassRuleDrawer
        hostId="h-1"
        mode="edit"
        open={true}
        onOpenChange={() => {}}
        rule={{
          id: "r-keyword",
          host_id: "h-1",
          rule_type: "domain_keyword",
          value: "abcde",
          is_risky: false,
          note: null,
          created_at: "",
          updated_at: "",
        }}
      />,
    );

    const valueInput = await screen.findByLabelText("值 *");
    await user.clear(valueInput);
    await user.type(valueInput, "abc");

    expect(
      await screen.findByText(/关键词较短，可能误命中其他域名/i),
    ).toBeInTheDocument();
  });
});
