// The Import workspace panel — a placeholder task-view scaffold this round. The
// real enter-act-leave import flow (source pick → progress → summary) is task 38
// (frontend-import epic); task 37 only stands up the tab that reaches it. Unlike
// the Catalog panel, this one is NOT force-mounted: a task view owns its transient
// state privately (C3) and may reset on each entry.

import { useTranslation } from "react-i18next";
import s from "./import-panel.module.css";

export function ImportPanel() {
    const { t } = useTranslation();
    return (
        <section className={s.import} data-testid="import-panel">
            <h1 className={s.title}>{t("import.title")}</h1>
            <p className={s.hint}>{t("import.placeholder")}</p>
        </section>
    );
}
