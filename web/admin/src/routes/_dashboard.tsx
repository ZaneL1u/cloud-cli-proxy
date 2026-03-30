import { createFileRoute, Outlet, redirect } from "@tanstack/react-router";
import { isAuthenticated, getRole } from "@/lib/auth";
import { Sidebar } from "@/components/layout/sidebar";
import { Topbar } from "@/components/layout/topbar";
import { SecurityDisclaimer } from "@/components/security-disclaimer";
import {
  OnboardingGuide,
  useOnboardingGuide,
} from "@/components/onboarding-guide";

export const Route = createFileRoute("/_dashboard")({
  beforeLoad: () => {
    if (!isAuthenticated()) {
      throw redirect({ to: "/login" });
    }
    const role = getRole();
    if (role !== "admin") {
      throw redirect({ to: "/portal" });
    }
  },
  component: DashboardLayout,
});

function DashboardLayout() {
  const guide = useOnboardingGuide();

  return (
    <div className="flex h-screen">
      <SecurityDisclaimer />
      <OnboardingGuide
        role="admin"
        forceOpen={guide.forceOpen}
        onClose={guide.onClose}
      />
      <Sidebar />
      <div className="flex flex-1 flex-col overflow-hidden">
        <Topbar onHelpClick={guide.openGuide} />
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
    </div>
  );
}
