// Job progress is ephemeral chrome, NOT query-cache state (_project-tracking/seam/02-events-jobs-and-binary.md). This
// hook subscribes to the push events and holds the in-flight jobs in local
// state. StatusBar consumes it; the import summary modal will too.

import { useEffect, useState } from "react";
import { log } from "@/lib/logger";
import { api } from "@/api/queries";
import type { JobDone, JobProgress } from "@/api/contract";

export interface ActiveJob {
    jobId: string;
    kind: JobProgress["kind"];
    done: number;
    total: number;
    stage?: string;
}

export function useJobs(): ActiveJob[] {
    const [jobs, setJobs] = useState<Map<string, ActiveJob>>(new Map());

    useEffect(() => {
        const offProgress = api.onJobProgress((p: JobProgress) => {
            setJobs((prev) => new Map(prev).set(p.jobId, p));
        });
        const offDone = api.onJobDone((d: JobDone) => {
            if (d.error) log.error("job failed", { jobId: d.jobId, kind: d.kind, error: d.error });
            setJobs((prev) => {
                const next = new Map(prev);
                next.delete(d.jobId);
                return next;
            });
        });
        return () => {
            offProgress();
            offDone();
        };
    }, []);

    return [...jobs.values()];
}
