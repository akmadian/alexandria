import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type ReactNode, useEffect } from "react";
import { startEventPump } from "@/api/event-pump";

// TanStack defaults per frontend/09: we own the freshness signal (the engine
// pushes C8 events → targeted invalidation), so focus/reconnect refetching is off
// and staleTime is long. Reads don't auto-retry by default (local IPC, not a
// network); transient codes opt in per-query. Mutations never auto-retry here.
const queryClient = new QueryClient({
    defaultOptions: {
        queries: {
            staleTime: 5 * 60_000,
            gcTime: 10 * 60_000,
            refetchOnWindowFocus: false,
            refetchOnReconnect: false,
            retry: false,
        },
        mutations: { retry: false },
    },
});

export function Providers({ children }: { children: ReactNode }) {
    // The event pump is THE one subscriber to the C8 stream, mounted where the
    // QueryClient lives: catalog events invalidate this client, jobs events land
    // in the jobs store. The effect's cleanup tears the subscription down.
    useEffect(() => startEventPump(queryClient), []);
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}
