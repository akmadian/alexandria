// NoticeRegion — the visible half of the minimal notice floor (stores/notices.ts).
// Renders the live notices as a stack of dismissible messages so a failed write is
// a rendered state, never a silent one (task 34: loud failure). Messages are i18n
// keys resolved here (C14). aria-live=assertive so a failure is announced.
//
// ponytail: a floor, not the toast SYSTEM — no positioning grammar, motion,
// severity variants, or queue policy (see stores/notices.ts for the ceiling).

import { useTranslation } from "react-i18next";
import { Button } from "@/components/button/button";
import { useDismissNotice, useNotices } from "@/stores/notices";
import styles from "./notice-region.module.css";

export function NoticeRegion() {
    const { t } = useTranslation();
    const notices = useNotices();
    const dismiss = useDismissNotice();
    if (notices.length === 0) return null;
    return (
        <div className={styles.region} role="alert" aria-live="assertive">
            {notices.map((notice) => (
                <div key={notice.id} className={styles.notice}>
                    <span className={styles.message}>{t(notice.messageKey)}</span>
                    <Button rung="ghost" onPress={() => dismiss(notice.id)}>
                        {t("modal.close")}
                    </Button>
                </div>
            ))}
        </div>
    );
}
