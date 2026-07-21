// Notices — the minimal LOUD-failure surface (task 34). A write that fails after
// its quiet retries rolls back the optimistic cache and pushes a notice here; the
// NoticeRegion renders it as a visible state so a failure is never silent
// (frontend-architecture §Optimistic mutation: "revert patched rows + toast —
// loud, never silent"). Messages are i18n KEYS resolved at render (C14).
//
// ponytail: this is a floor, not the toast/notice SYSTEM. No stacking policy,
// positioning grammar, motion, severity variants, dedup, or action affordances —
// the notice round owns those. Ceiling: when a second notice source appears
// (jobs/sync/watcher toasts, per frontend-architecture §Event pump), promote this
// to a real primitive with those concerns. For now: push a key, auto-expire,
// dismiss by hand.

import { create } from "zustand";
import { log } from "@/lib/logger";

/** Auto-expiry so a burst of failures can't pile up unbounded on the floor. */
const EXPIRY_MS = 6000;

export interface Notice {
    id: string;
    /** i18n key, resolved by NoticeRegion (C14). */
    messageKey: string;
}

interface NoticeStore {
    notices: readonly Notice[];
    pushNotice: (messageKey: string) => void;
    dismissNotice: (id: string) => void;
}

const useNoticeStore = create<NoticeStore>((set, get) => ({
    notices: [],
    pushNotice: (messageKey) => {
        const id = crypto.randomUUID();
        set((state) => ({ notices: [...state.notices, { id, messageKey }] }));
        log.warn("notice raised", { messageKey });
        setTimeout(() => get().dismissNotice(id), EXPIRY_MS);
    },
    dismissNotice: (id) => set((state) => ({ notices: state.notices.filter((notice) => notice.id !== id) })),
}));

/** Raise a notice from non-React code (the mutation's onError). */
export function pushNotice(messageKey: string): void {
    useNoticeStore.getState().pushNotice(messageKey);
}

export const useNotices = (): readonly Notice[] => useNoticeStore((state) => state.notices);
export const useDismissNotice = (): ((id: string) => void) => useNoticeStore((state) => state.dismissNotice);
