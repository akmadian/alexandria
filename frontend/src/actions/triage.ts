// The first slice of the action system (frontend-architecture §Module structure;
// epic: frontend-keyboard-actions). An action is a registered verb; this sliver
// carries only the TRIAGE verbs and their grid-context key bindings — the palette,
// rebindable keybindings, contexts beyond `grid`, and the full registry entry
// shape (title/aliases/predicate) land with the keyboard epic proper.
//
// Each verb's payload is an ABSOLUTE-valued TriagePatch (frontend-architecture: no
// deltas) — "rate 3" is `{ rating: 3 }`, "clear rating" is `{ rating: null }`.
// Targeting (C5: selection if non-empty, else cursor) and the write itself are the
// caller's job (use-grid-keys.ts); the registry is pure data + a pure resolver.

import type { ColorLabel } from "@/_generated-types/enums";
import type { TriagePatch } from "@/api/contract";

export type TriageActionId =
    | "rate-0"
    | "rate-1"
    | "rate-2"
    | "rate-3"
    | "rate-4"
    | "rate-5"
    | "label-red"
    | "label-yellow"
    | "label-green"
    | "label-blue"
    | "label-clear"
    | "flag-pick"
    | "flag-reject"
    | "flag-clear";

export interface TriageAction {
    id: TriageActionId;
    /** i18n key for the verb's label (C14) — reused by aria-labels and, later, the palette. */
    titleKey: string;
    /** The absolute-valued patch this verb applies. */
    patch: TriagePatch;
}

const rating = (value: number | null): TriagePatch => ({ rating: value });
const label = (value: ColorLabel | null): TriagePatch => ({ colorLabel: value });

// C10: `satisfies Record<TriageActionId, …>` makes the registry exhaustive — a new
// action id fails to compile until it has an entry, and every entry is keyed by
// its own id (the two-place invariant the completeness trick guarantees).
export const triageActions = {
    "rate-0": { id: "rate-0", titleKey: "actions.rate_0", patch: rating(null) },
    "rate-1": { id: "rate-1", titleKey: "actions.rate_1", patch: rating(1) },
    "rate-2": { id: "rate-2", titleKey: "actions.rate_2", patch: rating(2) },
    "rate-3": { id: "rate-3", titleKey: "actions.rate_3", patch: rating(3) },
    "rate-4": { id: "rate-4", titleKey: "actions.rate_4", patch: rating(4) },
    "rate-5": { id: "rate-5", titleKey: "actions.rate_5", patch: rating(5) },
    "label-red": { id: "label-red", titleKey: "actions.label_red", patch: label("red") },
    "label-yellow": { id: "label-yellow", titleKey: "actions.label_yellow", patch: label("yellow") },
    "label-green": { id: "label-green", titleKey: "actions.label_green", patch: label("green") },
    "label-blue": { id: "label-blue", titleKey: "actions.label_blue", patch: label("blue") },
    "label-clear": { id: "label-clear", titleKey: "actions.label_clear", patch: label(null) },
    "flag-pick": { id: "flag-pick", titleKey: "actions.flag_pick", patch: { flag: "pick" } },
    "flag-reject": { id: "flag-reject", titleKey: "actions.flag_reject", patch: { flag: "reject" } },
    "flag-clear": { id: "flag-clear", titleKey: "actions.clear_flag", patch: { flag: null } },
} as const satisfies Record<TriageActionId, TriageAction>;

// The grid context's (key → action) table — the LrC grammar (epic §Verb grammar):
// 0–5 rate (0 clears), 6–9 label (− clears), P pick / X reject / U clear. Keys are
// the NORMALIZED KeyboardEvent.key (letters lowercased); digits and "-" pass
// through. A key with no triage verb resolves to null and the dispatcher lets the
// event fall through (grid navigation etc.).
const gridTriageKeymap = {
    "0": "rate-0",
    "1": "rate-1",
    "2": "rate-2",
    "3": "rate-3",
    "4": "rate-4",
    "5": "rate-5",
    "6": "label-red",
    "7": "label-yellow",
    "8": "label-green",
    "9": "label-blue",
    "-": "label-clear",
    p: "flag-pick",
    x: "flag-reject",
    u: "flag-clear",
} as const satisfies Record<string, TriageActionId>;

/** Resolve a normalized key to its triage action in the grid context, or null. */
export function resolveGridTriageAction(key: string): TriageAction | null {
    const normalized = key.length === 1 ? key.toLowerCase() : key;
    const id = (gridTriageKeymap as Record<string, TriageActionId | undefined>)[normalized];
    return id === undefined ? null : triageActions[id];
}
