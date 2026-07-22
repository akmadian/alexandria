// The Catalog workspace panel: the grid well (§2), the inspector rail (§12's
// right zone), and the status bar (§19). This is the old shell body, lifted into
// a tab panel so the header could become the workspace tab strip (task 37). It
// stays MOUNTED when another tab is active (workspace.tsx force-mounts it), so
// C3 holds for free — query, selection, cursor, and grid scroll survive a trip
// through Import and back.
//
// The library title retired with the header: the Catalog tab now names the space,
// and the asset-count metric moved here to the status bar's left zone. The
// FilterBar (Catalog-space chrome, frontend-import epic) lands as this panel's top
// row in its own round.

import { useTranslation } from "react-i18next";
import { useAssetTotal } from "@/api/queries";
import { PaneErrorBoundary } from "@/components/error-boundary/error-boundary";
import { NoticeRegion } from "@/components/notice/notice-region";
import { Grid } from "@/features/grid/grid";
import { Inspector } from "@/features/inspector/inspector";
import { formatNumber } from "@/lib/format";
import { useCatalogQuery, useSelectionCount } from "@/stores/catalog-store";
import s from "./catalog-panel.module.css";

export function CatalogPanel() {
    const { t } = useTranslation();
    const { query, arrangement } = useCatalogQuery();
    const total = useAssetTotal(query, arrangement);
    const selected = useSelectionCount(total ?? 0);

    return (
        <div className={s.catalog} data-testid="catalog-panel">
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
                {total !== undefined && (
                    <span className={s.metric}>
                        {t("shell.assets", { count: total, formatted: formatNumber(total) })}
                    </span>
                )}
                {selected > 0 && (
                    <span className={s.metric}>
                        {t("statusBar.selected", { count: selected, formatted: formatNumber(selected) })}
                    </span>
                )}
            </footer>
            <NoticeRegion />
        </div>
    );
}
