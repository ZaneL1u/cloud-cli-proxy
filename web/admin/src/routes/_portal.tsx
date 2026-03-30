import { createFileRoute, Outlet, redirect } from "@tanstack/react-router";
import { isAuthenticated } from "@/lib/auth";
import { Topbar } from "@/components/layout/topbar";
import { SecurityDisclaimer } from "@/components/security-disclaimer";

export const Route = createFileRoute("/_portal")({
  beforeLoad: () => {
    if (!isAuthenticated()) {
      throw redirect({ to: "/login" });
    }
  },
  component: PortalLayout,
});

function PortalLayout() {
  return (
    <div className="flex h-screen flex-col">
      <SecurityDisclaimer />
      <Topbar />
      <main className="flex-1 overflow-y-auto bg-muted/40 p-6">
        <div className="min-h-[calc(100%-3rem)]">
          <Outlet />
        </div>
        <footer className="mt-8 border-t pt-4 pb-2 text-center text-xs text-muted-foreground">
          Powered by{" "}
          <a
            href="https://github.com/zanel1u/cloud-cli-proxy"
            target="_blank"
            rel="noopener noreferrer"
            className="font-medium text-primary hover:underline"
          >
            cloud-cli-proxy
          </a>
        </footer>
      </main>
    </div>
  );
}
