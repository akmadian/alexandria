import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { installGlobalCapture, recentLogs } from "@/lib/logger";
import { Button } from "@/components/button/button";
import { ErrorBoundary } from "./error-boundary";
import { LibraryProvider } from "./library-state";
import { Shell } from "./shell";

installGlobalCapture();

// Query defaults tuned for a local desktop app: cheap to re-fetch (sub-10ms IPC),
// pointless on window focus; small gcTime keeps the list cache a browsing window,
// not a DB mirror. Reads auto-retry once (seam error shape: _project-tracking/seam/02-events-jobs-and-binary.md); writes never.
function makeClient() {
    return new QueryClient({
        defaultOptions: {
            queries: { refetchOnWindowFocus: false, staleTime: 30_000, gcTime: 5 * 60_000, retry: 1 },
            mutations: { retry: 0 },
        },
    });
}

const CrashScreen = ({ error }: { error: Error }) => {
    const { t } = useTranslation();
    const copy = () => {
        void navigator.clipboard.writeText(JSON.stringify({ error: { message: error.message, stack: error.stack }, logs: recentLogs().slice(-200) }, null, 2));
    };
    return (
        <div style={{ display: "grid", placeItems: "center", height: "100dvh", textAlign: "center" }}>
            <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)", maxWidth: 420 }}>
                <h1 style={{ fontSize: "var(--text-lg)" }}>{t("errors.appCrashed")}</h1>
                <p style={{ color: "var(--text-tertiary)", fontSize: "var(--text-sm)" }}>{t("errors.restartHint")}</p>
                <Button variant="primary" onPress={copy}>
                    {t("errors.copyDetails")}
                </Button>
            </div>
        </div>
    );
};

export const App = () => {
    const [client] = useState(makeClient);
    return (
        <ErrorBoundary fallback={(error) => <CrashScreen error={error} />}>
            <QueryClientProvider client={client}>
                <LibraryProvider>
                    <Shell />
                </LibraryProvider>
            </QueryClientProvider>
        </ErrorBoundary>
    );
};
