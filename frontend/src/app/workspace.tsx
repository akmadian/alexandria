// The workspace shell (frontend-import epic amendment, 2026-07-21): navigation
// between views works like BROWSER TABS — a persistent slim header strip of tabs
// plus a settings gear in the corner; selecting a tab swaps everything below. The
// FilterBar is Catalog-space chrome, not global, so the header holds ONLY the tab
// strip and the gear (the epic's amendment §3).
//
// C3 is load-bearing here: switching away from Catalog and back must restore it
// exactly (query, selection, cursor, scroll). RAC's `shouldForceMount` keeps the
// Catalog panel in the DOM while inactive (inert + hidden, not unmounted), so the
// virtualizer's scroll state — component-local — survives for free. Import is a
// task view (C3): it owns transient state privately and is NOT force-mounted.

import type { ReactNode } from "react";
import {
    Button as AriaButton,
    Tab,
    TabList,
    TabPanel,
    Tabs,
    type Key,
} from "react-aria-components";
import { useTranslation } from "react-i18next";
import { Icon } from "@/components/icon/icon";
import { CatalogPanel } from "./catalog-panel";
import { ImportPanel } from "./import-panel";
import s from "./workspace.module.css";

// The tab registry (C10): key → its label + panel + keep-mounted policy. The
// `satisfies Record` gate makes a new tab a compile error until it declares all
// three fields. `keepMounted` is the C3 lever — Catalog stays resident, task views
// do not. Review joins as a row when its epic lands (frontend-import epic, "out of
// scope"); adding it is one entry here plus one line in TAB_ORDER, zero new UI.
type WorkspaceTabKey = "catalog" | "import";

interface WorkspaceTab {
    /** i18n key for the tab label (C14). */
    labelKey: string;
    /** The surface swapped in below the strip when this tab is selected. */
    Panel: () => ReactNode;
    /** C3: force-mount and keep in the DOM when inactive (Catalog), vs. mount on
     * entry (task views own transient state privately). */
    keepMounted: boolean;
}

const WORKSPACE_TABS = {
    catalog: { labelKey: "workspace.tab.catalog", Panel: CatalogPanel, keepMounted: true },
    import: { labelKey: "workspace.tab.import", Panel: ImportPanel, keepMounted: false },
} as const satisfies Record<WorkspaceTabKey, WorkspaceTab>;

// The render order of the strip. Kept beside the registry so the two stay in
// lockstep (a completeness test pins it).
const TAB_ORDER: readonly WorkspaceTabKey[] = ["catalog", "import"];

export function Workspace() {
    const { t } = useTranslation();

    return (
        // Uncontrolled: the strip owns its own selection; the design-library hash
        // route (app.tsx) is a separate surface, not a tab, so nothing syncs here.
        // keyboardActivation defaults to "automatic" — arrow keys move AND select.
        <Tabs className={s.shell} defaultSelectedKey={"catalog" satisfies Key}>
            <header className={s.header}>
                <TabList className={s.tabList} aria-label={t("workspace.tabsLabel")}>
                    {TAB_ORDER.map((key) => (
                        <Tab key={key} id={key} className={s.tab}>
                            {t(WORKSPACE_TABS[key].labelKey)}
                        </Tab>
                    ))}
                </TabList>
                {/* ponytail: an inert placeholder this round — renders and focuses,
                 * does nothing on press. The settings task view is a separate round
                 * (frontend-import epic amendment §2); wiring onPress opens it. Left
                 * enabled so it stays in the tab order (a disabled button is not
                 * focusable). */}
                <AriaButton className={s.gear} aria-label={t("workspace.settings")}>
                    <Icon concept="settings" />
                </AriaButton>
            </header>
            {TAB_ORDER.map((key) => {
                const { Panel, keepMounted } = WORKSPACE_TABS[key];
                return (
                    <TabPanel key={key} id={key} className={s.panel} shouldForceMount={keepMounted || undefined}>
                        <Panel />
                    </TabPanel>
                );
            })}
        </Tabs>
    );
}

// Exported for the completeness test — the registry's keys and the render order
// must name exactly the same tabs.
export { TAB_ORDER, WORKSPACE_TABS };
export type { WorkspaceTabKey };
