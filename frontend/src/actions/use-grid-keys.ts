// The grid-context key dispatcher (task 34). A window-level keydown listener that
// turns a triage key into a write against the C5 target — active whenever the grid
// is mounted (its owner installs it). Triage keys are library-wide muscle memory
// in LrC, so this listens on window rather than a focused element, but it defers to
// text entry (a note field must swallow its own keystrokes) and to modified keys
// (Cmd/Ctrl/Alt belong to other shortcuts).

import { useEffect } from "react";
import { useUpdateAssets } from "@/api/mutations";
import { log } from "@/lib/logger";
import { readTriageTargetIds } from "@/stores/catalog-store";
import { resolveGridTriageAction } from "./triage";

/** A keystroke aimed at text entry is the field's, never a verb's. */
function isTextEntry(target: EventTarget | null): boolean {
    if (!(target instanceof HTMLElement)) return false;
    return target.isContentEditable || target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.tagName === "SELECT";
}

/**
 * Install the grid-context triage keys for as long as the caller is mounted.
 * Returns nothing — the effect is the whole surface. Kept thin: the keymap and the
 * targeting rule are pure and tested in isolation; this only guards the event,
 * resolves the target, and fires the write.
 */
export function useGridKeys(): void {
    const { writeTriage } = useUpdateAssets();
    useEffect(() => {
        function onKeyDown(event: KeyboardEvent): void {
            if (event.metaKey || event.ctrlKey || event.altKey || isTextEntry(event.target)) return;
            const action = resolveGridTriageAction(event.key);
            if (action === null) return;

            const ids = readTriageTargetIds();
            if (ids === null) {
                log.debug("triage key ignored: all-shaped selection gated until the undo round", { action: action.id });
                return;
            }
            if (ids.length === 0) return;

            event.preventDefault();
            log.debug("triage key", { action: action.id, targets: ids.length });
            writeTriage({ ids }, action.patch);
        }
        window.addEventListener("keydown", onKeyDown);
        return () => window.removeEventListener("keydown", onKeyDown);
    }, [writeTriage]);
}
