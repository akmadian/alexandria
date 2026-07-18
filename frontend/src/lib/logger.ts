// Frontend logging (docs/functional-requirements.md §Logging and Observability).
//
// Every entry goes to an in-memory ring buffer and a batch queue. The queue
// flushes to a pluggable sink — today a dev-console no-op; when the backend
// binds, the sink becomes the `logBatch` binding so frontend entries land in
// the Go rotating log and one file tells the whole interleaved story.
//
// ponytail: no levels config, no transports, no sampling. A local app's
// observability need is post-hoc debugging; a ring buffer + one sink covers it.

export type LogLevel = "debug" | "info" | "warn" | "error";

export interface LogEntry {
    ts: string; // ISO
    level: LogLevel;
    msg: string;
    fields?: Record<string, unknown>;
}

const RING_SIZE = 2000;
const FLUSH_MS = 5000;

const sessionId = crypto.randomUUID().slice(0, 8);
const ring: LogEntry[] = [];
let queue: LogEntry[] = [];
let sink: (batch: LogEntry[]) => void | Promise<void> = () => {};
let flushTimer: ReturnType<typeof setInterval> | undefined;

/** Wire the backend sink (the logBatch binding) when it exists. */
export function setLogSink(fn: typeof sink): void {
    sink = fn;
}

/** Last N entries — the app-crash screen offers these as "copy error details". */
export function recentLogs(): readonly LogEntry[] {
    return ring;
}

function write(level: LogLevel, msg: string, fields?: Record<string, unknown>): void {
    const entry: LogEntry = { ts: new Date().toISOString(), level, msg, fields: { ...fields, sessionId } };
    ring.push(entry);
    if (ring.length > RING_SIZE) ring.shift();
    queue.push(entry);
    if (import.meta.env.DEV) {
        console[level === "debug" ? "log" : level](`[${level}] ${msg}`, fields ?? "");
    }
    if (level === "error") flush();
}

export function flush(): void {
    if (queue.length === 0) return;
    const batch = queue;
    queue = [];
    void sink(batch);
}

export const log = {
    debug: (msg: string, fields?: Record<string, unknown>) => write("debug", msg, fields),
    info: (msg: string, fields?: Record<string, unknown>) => write("info", msg, fields),
    warn: (msg: string, fields?: Record<string, unknown>) => write("warn", msg, fields),
    error: (msg: string, fields?: Record<string, unknown>) => write("error", msg, fields),
};

/** Mount once at app start: global error capture + periodic flush. */
export function installGlobalCapture(): void {
    window.addEventListener("error", (e) => log.error("uncaught error", { message: e.message, source: e.filename, line: e.lineno }));
    window.addEventListener("unhandledrejection", (e) => log.error("unhandled rejection", { reason: String(e.reason) }));
    window.addEventListener("pagehide", flush);
    flushTimer ??= setInterval(flush, FLUSH_MS);
}
