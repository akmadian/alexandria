import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";

// One client for the app. Defaults tuned for a local desktop app: data is cheap
// to re-fetch (sub-10ms IPC) but pointless to re-fetch on window focus, and a
// small gcTime keeps the list cache a browsing window, not a DB mirror (§7).
function makeClient() {
    return new QueryClient({
        defaultOptions: {
            queries: {
                refetchOnWindowFocus: false,
                staleTime: 30_000,
                gcTime: 5 * 60_000,
                retry: 1, // reads auto-retry once (§9); mutations never
            },
            mutations: { retry: 0 },
        },
    });
}

export const QueryProvider = ({ children }: { children: ReactNode }) => {
    const [client] = useState(makeClient);
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
};
