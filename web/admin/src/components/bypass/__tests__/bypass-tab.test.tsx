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

  it("渲染标题 + 预设规则集 + 自定义规则区", async () => {
    apiFetchMock.mockImplementation(async (path: string) => {
      if (path === "/bypass/presets") return { presets: [] };
      if (path.includes("/bindings")) return { bindings: [] };
      if (path.includes("/rules")) return { rules: [] };
      return {};
    });

    renderWithClient(<BypassTab hostId="h-1" />);

    expect(await screen.findByText("代理白名单")).toBeInTheDocument();
    expect(screen.getByText("预设规则集")).toBeInTheDocument();
    expect(screen.getByText("自定义规则")).toBeInTheDocument();
  });

  it("有规则时显示「N 条规则」徽章", async () => {
    apiFetchMock.mockImplementation(async (path: string) => {
      if (path === "/bypass/presets") return { presets: [] };
      if (path.includes("/bindings")) return { bindings: [] };
      if (path.includes("/rules"))
        return {
          rules: [
            {
              id: "r1",
              host_id: "h-1",
              rule_type: "ip",
              value: "1.2.3.4",
              port: null,
              is_risky: false,
              note: null,
              created_at: "",
              updated_at: "",
            },
          ],
        };
      return {};
    });

    renderWithClient(<BypassTab hostId="h-1" />);
    expect(await screen.findByText("1 条规则")).toBeInTheDocument();
  });
});
