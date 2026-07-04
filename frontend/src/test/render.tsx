// renderWithApp — the ~1 helper component tests use. Mounts the same providers
// the real app does (Query + LibraryProvider), against the mock seam that
// api/queries already points at. Tests assert behavior, not markup.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, type RenderOptions } from "@testing-library/react";
import type { ReactElement, ReactNode } from "react";
import { LibraryProvider } from "@/app/library-state";

export function renderWithApp(ui: ReactElement, options?: RenderOptions) {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    const Wrapper = ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>
            <LibraryProvider>{children}</LibraryProvider>
        </QueryClientProvider>
    );
    return render(ui, { wrapper: Wrapper, ...options });
}
