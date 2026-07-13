// The rebuild's app root (frontend/09). Slice 1: a minimal three-row shell —
// header, the grid on its canvas well, a status bar — running entirely against
// the mock catalog (no Wails, no Go). Panes, sidebar, filter bar, inspector, and
// the other view modes layer in from here.
//
// ponytail: user-facing chrome strings ("Library", "selected") are literals for
// now — they become i18n keys (C14) when the shell adopts the i18n scaffolding.
// Data values (counts) already go through Intl.

import { useSyncExternalStore } from "react";
import { useQueryAssets } from "@/api/queries";
import { DesignLibrary } from "@/features/design-library/design-library";
import { FilterBar } from "@/features/filter-bar/filter-bar";
import { Grid } from "@/features/grid/grid";
import { formatNumber } from "@/lib/format";
import { useCatalogQuery, useSelectionCount } from "@/stores/catalog-store";
import s from "./app.module.css";
import { Providers } from "./providers";

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

function Shell() {
    const { query, arrangement } = useCatalogQuery();
    const { data } = useQueryAssets(query, arrangement);
    const total = data?.total ?? 0;
    const selected = useSelectionCount(total);

    return (
        <div className={s.shell}>
            <header className={s.header}>
                <span className={s.title}>Library</span>
                {data && <span className={s.metric}>{formatNumber(total)} assets</span>}
            </header>
            <FilterBar />
            <main className={s.main}>
                <Grid />
            </main>
            <footer className={s.status}>
                <span className={s.metric}>{selected > 0 ? `${formatNumber(selected)} selected` : "—"}</span>
            </footer>
        </div>
    );
}

export function App() {
    const hash = useHash();
    return (
        <Providers>
            {hash.startsWith("#/design-library") ? <DesignLibrary /> : <Shell />}
        </Providers>
    );
}
