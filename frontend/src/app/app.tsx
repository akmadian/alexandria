// The rebuild's app root (frontend/09). The shell is the workspace tab strip
// (Catalog + Import, task 37) over the active panel — running against the mock
// catalog under `bun run dev`, the real engine under `wails dev`. The design
// library remains reachable at #/design-library.

import { useSyncExternalStore } from "react";
import { DesignLibrary } from "@/features/design-library/design-library";
import { Providers } from "./providers";
import { Workspace } from "./workspace";

// ponytail: hash check instead of a router — one alternate page doesn't justify
// a routing dep; revisit when the app grows real routes.
function useHash(): string {
    return useSyncExternalStore(
        (notify) => {
            window.addEventListener("hashchange", notify);
            return () => window.removeEventListener("hashchange", notify);
        },
        () => window.location.hash,
    );
}

export function App() {
    const hash = useHash();
    return (
        <Providers>
            {hash.startsWith("#/design-library") ? <DesignLibrary /> : <Workspace />}
        </Providers>
    );
}
