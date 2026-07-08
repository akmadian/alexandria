// Keybindings (docs/project-tracking/frontend/04-keyboard-and-actions.md).
//
// Ownership: the FRONTEND defines the action vocabulary, default combos, and
// contexts; the backend is persistence for user overrides only. Overrides live
// in localStorage until the getKeybindingOverrides/saveKeybindingOverrides
// bindings exist — swapping the store is confined to loadOverrides/saveOverrides.
//
// Combo grammar: lowercase, "+"-joined, modifiers sorted mod<shift<alt, e.g.
// "5", "p", "mod+z", "mod+shift+z". "mod" = ⌘ on macOS, Ctrl elsewhere.

export type KeyContext = "global" | "grid" | "detail" | "import";

export interface ActionDef {
    id: string;
    context: KeyContext;
    defaultCombo: string;
    labelKey: string; // i18n key — shown in settings UI and command palette
}

/** The action vocabulary. Add actions here; features register handlers for them. */
export const ACTIONS: readonly ActionDef[] = [
    { id: "rate_0", context: "grid", defaultCombo: "0", labelKey: "actions.rate_0" },
    { id: "rate_1", context: "grid", defaultCombo: "1", labelKey: "actions.rate_1" },
    { id: "rate_2", context: "grid", defaultCombo: "2", labelKey: "actions.rate_2" },
    { id: "rate_3", context: "grid", defaultCombo: "3", labelKey: "actions.rate_3" },
    { id: "rate_4", context: "grid", defaultCombo: "4", labelKey: "actions.rate_4" },
    { id: "rate_5", context: "grid", defaultCombo: "5", labelKey: "actions.rate_5" },
    { id: "flag_pick", context: "grid", defaultCombo: "p", labelKey: "actions.flag_pick" },
    { id: "flag_reject", context: "grid", defaultCombo: "x", labelKey: "actions.flag_reject" },
    { id: "clear_flag", context: "grid", defaultCombo: "u", labelKey: "actions.clear_flag" },
    { id: "select_all", context: "grid", defaultCombo: "mod+a", labelKey: "actions.select_all" },
    { id: "clear_selection", context: "grid", defaultCombo: "escape", labelKey: "actions.clear_selection" },
    { id: "command_palette", context: "global", defaultCombo: "mod+k", labelKey: "actions.command_palette" },
];

const byId = new Map(ACTIONS.map((a) => [a.id, a]));

// --- overrides -------------------------------------------------------------

const OVERRIDES_KEY = "alexandria.keybindings";
let overrides: Record<string, string> = loadOverrides();

function loadOverrides(): Record<string, string> {
    try {
        return JSON.parse(localStorage.getItem(OVERRIDES_KEY) ?? "{}") as Record<string, string>;
    } catch {
        return {};
    }
}

export function comboFor(actionId: string): string | undefined {
    return overrides[actionId] ?? byId.get(actionId)?.defaultCombo;
}

/** Returns the conflicting action id if `combo` is already taken in a reachable context, else sets the override. */
export function setOverride(actionId: string, combo: string): string | null {
    const conflict = findConflict(actionId, combo);
    if (conflict) return conflict;
    overrides = { ...overrides, [actionId]: combo };
    localStorage.setItem(OVERRIDES_KEY, JSON.stringify(overrides));
    return null;
}

export function resetOverrides(): void {
    overrides = {};
    localStorage.removeItem(OVERRIDES_KEY);
}

/** Synchronous conflict check against the effective map — no backend round-trip. */
export function findConflict(actionId: string, combo: string): string | null {
    const ctx = byId.get(actionId)?.context;
    for (const a of ACTIONS) {
        if (a.id === actionId) continue;
        // A combo conflicts if the other action is reachable at the same time:
        // same context, or either side is global.
        if (comboFor(a.id) === combo && (a.context === ctx || a.context === "global" || ctx === "global")) return a.id;
    }
    return null;
}

// --- normalization & dispatch ------------------------------------------------

const IS_MAC = typeof navigator !== "undefined" && navigator.platform.toLowerCase().includes("mac");

/** KeyboardEvent → combo string, or null for bare-modifier presses. */
export function normalizeCombo(e: Pick<KeyboardEvent, "key" | "metaKey" | "ctrlKey" | "shiftKey" | "altKey">): string | null {
    const key = e.key.toLowerCase();
    if (["shift", "control", "meta", "alt"].includes(key)) return null;
    const mod = IS_MAC ? e.metaKey : e.ctrlKey;
    const parts: string[] = [];
    if (mod) parts.push("mod");
    if (e.shiftKey && key.length > 1) parts.push("shift"); // printable chars already reflect shift ("5" vs "%")
    if (e.altKey) parts.push("alt");
    parts.push(key === " " ? "space" : key);
    return parts.join("+");
}

type Handler = () => void;
const handlers = new Map<string, Handler>();

/** Features register handlers on mount; returns the unregister cleanup. */
export function registerHandlers(map: Record<string, Handler>): () => void {
    for (const [id, fn] of Object.entries(map)) {
        if (!byId.has(id)) throw new Error(`unknown action: ${id}`); // typo guard — actions are defined in ACTIONS
        handlers.set(id, fn);
    }
    return () => {
        for (const id of Object.keys(map)) handlers.delete(id);
    };
}

export function resolve(context: KeyContext, combo: string): ActionDef | null {
    for (const a of ACTIONS) {
        if ((a.context === context || a.context === "global") && comboFor(a.id) === combo) return a;
    }
    return null;
}

function isEditable(el: EventTarget | null): boolean {
    if (!(el instanceof HTMLElement)) return false;
    return el.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName);
}

/** Mount once at the app root. `getContext` derives the active context from app state. */
export function installKeyboardDispatch(getContext: () => KeyContext): () => void {
    const onKeyDown = (e: KeyboardEvent) => {
        const combo = normalizeCombo(e);
        if (!combo) return;
        // Typing wins: inside editable elements only global (modifier-bearing) bindings fire.
        const action = resolve(getContext(), combo);
        if (!action) return;
        if (isEditable(e.target) && action.context !== "global") return;
        const fn = handlers.get(action.id);
        if (!fn) return;
        e.preventDefault();
        fn();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
}
