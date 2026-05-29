import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { PreviewSheet } from "../preview-sheet";
import type { BypassPreviewResponse } from "@/lib/api/types/bypass";

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

function makePreview(
  override: Partial<BypassPreviewResponse> = {},
): BypassPreviewResponse {
  return {
    config_hash: "abcdef1234",
    version_current: 23,
    version_next: 24,
    whitelist_cidrs_rendered: { version: 3, rules: [{ ip_cidr: ["10.0.0.0/8"] }] },
    whitelist_domains_rendered: { version: 3, rules: [{ domain: ["a.com"] }] },
    nft_diff: "+ 10.0.0.0/8\n- 192.168.1.0/24",
    risky_count: 0,
    summary: "覆盖 12 条规则",
    ...override,
  };
}

describe("PreviewSheet", () => {
  beforeEach(() => {
    apiFetchMock.mockReset();
  });

  it("打开 Sheet 自动调 preview mutation，加载完成后渲染版本号摘要", async () => {
    apiFetchMock.mockResolvedValueOnce(makePreview());
    renderWithClient(
      <PreviewSheet hostId="h-1" open={true} onOpenChange={() => {}} />,
    );

    expect(await screen.findByText("预览生效配置")).toBeInTheDocument();
    await waitFor(() => {
      expect(
        screen.getByText(/v23 → v24 · 覆盖 12 条规则/),
      ).toBeInTheDocument();
    });
    expect(apiFetchMock).toHaveBeenCalledWith(
      "/hosts/h-1/bypass/preview",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("双 Tab 「sing-box JSON」/「nft set diff」均渲染", async () => {
    apiFetchMock.mockResolvedValueOnce(makePreview());
    renderWithClient(
      <PreviewSheet hostId="h-1" open={true} onOpenChange={() => {}} />,
    );

    expect(await screen.findByText("sing-box JSON")).toBeInTheDocument();
    expect(screen.getByText("nft set diff")).toBeInTheDocument();
  });

  it("preview 失败显示错误占位 + toast.error 被触发", async () => {
    const { toast } = await import("sonner");
    apiFetchMock.mockRejectedValueOnce(
      new Error(JSON.stringify({ code: "BYPASS_HOST_UNREACHABLE", message: "x" })),
    );
    renderWithClient(
      <PreviewSheet hostId="h-1" open={true} onOpenChange={() => {}} />,
    );

    await waitFor(() => {
      expect(screen.getByTestId("preview-error")).toBeInTheDocument();
    });
    expect(toast.error).toHaveBeenCalledWith(
      "host-agent 当前不可达，配置已保存但未生效，将自动重试",
    );
  });
});
