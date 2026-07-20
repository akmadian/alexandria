// The rebuild's app root (frontend/09). The shell is header, the grid well,
// the inspector rail (the first §12 zone beyond the well), and a status bar —
// running against the mock catalog under `bun run dev`, the real engine under
// `wails dev`. The browser rail and filter bar layer in with their feature
// rounds; the design library remains reachable at #/design-library.

import { useSyncExternalStore } from "react";
import { useTranslation } from "react-i18next";
import { useQueryAssets } from "@/api/queries";
import { PaneErrorBoundary } from "@/components/error-boundary/error-boundary";
import { DesignLibrary } from "@/features/design-library/design-library";
import { Grid } from "@/features/grid/grid";
import { Inspector } from "@/features/inspector/inspector";
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
    const { t } = useTranslation();
    const { query, arrangement } = useCatalogQuery();
    const { data } = useQueryAssets(query, arrangement);
    const total = data?.total ?? 0;
    const selected = useSelectionCount(total);

    return (
        <div className={s.shell}>
            <header className={s.header}>
                <span className={s.title}>{t("shell.library")}</span>
                {data && (
                    <span className={s.metric}>
                        {t("shell.assets", { count: total, formatted: formatNumber(total) })}
                    </span>
                )}
            </header>
            <main className={s.main}>
                <PaneErrorBoundary>
                    <Grid />
                </PaneErrorBoundary>
            </main>
            <aside className={s.rail}>
                <PaneErrorBoundary>
                    <Inspector />
                </PaneErrorBoundary>
            </aside>
            <footer className={s.status}>
                <span className={s.metric}>
                    {selected > 0
                        ? t("statusBar.selected", { count: selected, formatted: formatNumber(selected) })
                        : "—"}
                </span>
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
