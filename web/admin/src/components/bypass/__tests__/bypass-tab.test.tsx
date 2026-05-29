import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BypassTab } from "../bypass-tab";

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

describe("BypassTab", () => {
  beforeEach(() => {
    apiFetchMock.mockReset();
  });

  it("渲染标题 + 警告 + 规则表 + 应用与查看配置按钮", async () => {
    apiFetchMock.mockImplementation(async (path: string) => {
      if (path === "/bypass/presets") return { presets: [] };
      if (path.startsWith("/hosts/") && path.endsWith("/bypass"))
        return { bindings: [] };
      if (path.startsWith("/bypass/rules")) return { rules: [] };
      return {};
    });

    renderWithClient(<BypassTab hostId="h-1" />);

    expect(await screen.findByText("代理白名单")).toBeInTheDocument();
    expect(screen.getByText("应用")).toBeInTheDocument();
    expect(screen.getByText("查看配置")).toBeInTheDocument();
  });

  it("有规则时显示规则总数", async () => {
    apiFetchMock.mockImplementation(async (path: string) => {
      if (path === "/bypass/presets") return { presets: [] };
      if (path.startsWith("/hosts/") && path.endsWith("/bypass"))
        return { bindings: [] };
      if (path.startsWith("/bypass/rules"))
        return {
          rules: [
            {
              id: "r1", host_id: "h-1", rule_type: "ip", value: "1.2.3.4",
              is_risky: false, note: null, created_at: "", updated_at: "",
            },
          ],
        };
      return {};
    });

    renderWithClient(<BypassTab hostId="h-1" />);
    expect(await screen.findByText("1 条")).toBeInTheDocument();
  });
});
