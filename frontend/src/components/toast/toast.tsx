// Toast store + outlet. Module-level store (useSyncExternalStore) so any code —
// mutations, the event bridge, non-React modules — can call toast() without
// threading context. Error-surface rules live in the seam doc §9: only
// "unexpected" failures earn a toast; degraded states get inline treatment.

import { X } from "lucide-react";
import { useSyncExternalStore } from "react";
import { Button } from "@/components/button/button";
import { Icon } from "@/components/icon/icon";
import s from "./toast.module.css";

export interface ToastItem {
    id: number;
    kind: "info" | "error";
    message: string;
    /** e.g. a manual Retry for a failed write — writes never auto-retry. */
    action?: { label: string; onPress: () => void };
}

let nextId = 1;
let items: ToastItem[] = [];
const listeners = new Set<() => void>();

function emit() {
    for (const l of listeners) l();
}

export function toast(kind: ToastItem["kind"], message: string, action?: ToastItem["action"]): void {
    const item: ToastItem = { id: nextId++, kind, message, action };
    items = [...items, item];
    emit();
    if (!action) setTimeout(() => dismiss(item.id), 5000);
}

export function dismiss(id: number): void {
    items = items.filter((t) => t.id !== id);
    emit();
}

const subscribe = (fn: () => void) => {
    listeners.add(fn);
    return () => listeners.delete(fn);
};

/** Mount once, in the shell. */
export const Toasts = () => {
    const current = useSyncExternalStore(subscribe, () => items);
    if (current.length === 0) return null;
    return (
        <div className={s.stack} role="status" aria-live="polite">
            {current.map((t) => (
                <div key={t.id} className={`${s.toast} ${t.kind === "error" ? s.error : ""}`}>
                    <span className={s.message}>{t.message}</span>
                    {t.action && (
                        <Button
                            size="sm"
                            onPress={() => {
                                t.action?.onPress();
                                dismiss(t.id);
                            }}
                        >
                            {t.action.label}
                        </Button>
                    )}
                    <Button size="sm" onPress={() => dismiss(t.id)} aria-label="Dismiss">
                        <Icon icon={X} size={12} />
                    </Button>
                </div>
            ))}
        </div>
    );
};
