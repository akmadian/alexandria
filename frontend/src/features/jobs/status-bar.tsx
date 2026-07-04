// Bottom chrome: selection count, active-job progress, total. Reads job push
// events via useJobs (not the query cache) and selection from LibraryProvider.

import { useTranslation } from "react-i18next";
import { useLibraryState } from "@/app/library-state";
import { useJobs } from "./use-jobs";
import s from "./status-bar.module.css";

export const StatusBar = ({ total }: { total: number }) => {
    const { t } = useTranslation();
    const { selection } = useLibraryState();
    const jobs = useJobs();

    return (
        <div className={s.bar}>
            {jobs.map((job) => (
                <span key={job.jobId} className={s.job}>
                    {t(`statusBar.job.${job.kind}`, { defaultValue: job.kind })}
                    <span className={s.progress}>
                        <span className={s.fill} style={{ width: `${job.total ? (job.done / job.total) * 100 : 0}%` }} />
                    </span>
                    <span className="u-data">
                        {job.done}/{job.total}
                    </span>
                </span>
            ))}
            <span className={s.spacer} />
            {selection.size > 0 && <span className="u-data">{t("statusBar.selected", { count: selection.size })}</span>}
            <span className={`${s.total} u-data`}>{t("filterBar.count", { count: total })}</span>
        </div>
    );
};
