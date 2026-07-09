// impl/14 walking skeleton — THROWAWAY. Proves the seam pipe end to end:
// Go SourceService.ListSources → Wails binding → generated TS → this page under
// `wails dev`. Deleted at the frontend ground-up rebuild (frontend/09); nothing
// should import it.
import { useEffect, useState } from "react";

import { ListSources } from "../wailsjs/go/seam/SourceService";
import type { domain } from "../wailsjs/go/models";

type State =
    | { status: "loading" }
    | { status: "error"; message: string }
    | { status: "ok"; sources: domain.Source[] };

export function SeamSkeleton() {
    const [state, setState] = useState<State>({ status: "loading" });

    useEffect(() => {
        ListSources()
            // A Go nil slice (empty catalog) serializes to null over the binding —
            // coalesce so "no sources" renders as a result, not perpetual loading.
            .then((sources) => setState({ status: "ok", sources: sources ?? [] }))
            .catch((cause: unknown) => setState({ status: "error", message: String(cause) }));
    }, []);

    return (
        <main style={{ fontFamily: "system-ui", padding: 24 }}>
            <h1>Alexandria — seam walking skeleton</h1>
            <p>
                Live call to <code>SourceService.ListSources()</code> over the Wails binding:
            </p>
            {state.status === "loading" && <p>loading…</p>}
            {state.status === "error" && <pre style={{ color: "crimson" }}>error: {state.message}</pre>}
            {state.status === "ok" && (
                <>
                    <p>{state.sources.length} source(s):</p>
                    <pre>{JSON.stringify(state.sources, null, 2)}</pre>
                </>
            )}
        </main>
    );
}
