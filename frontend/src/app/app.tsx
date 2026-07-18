// The rebuild's app root (frontend/09). The shell is deliberately minimal while
// primitives land: header, the (empty) grid well, a status bar — running against
// the mock catalog (no Wails, no Go). The grid and filter bar layer in with
// their feature rounds; until then the well points at the design library.
//
// ponytail: user-facing chrome strings ("Library", "selected") are literals for
// now — they become i18n keys (C14) when the shell adopts the i18n scaffolding.
// Data values (counts) already go through Intl.

import { useSyncExternalStore } from "react";
import { useQueryAssets } from "@/api/queries";
import { DesignLibrary } from "@/features/design-library/design-library";
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
            <main className={s.main}>
                <p className={s.wellHint}>
                    The grid arrives with its feature round —{" "}
                    <a className={s.wellLink} href="#/design-library">design library</a>
                </p>
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
