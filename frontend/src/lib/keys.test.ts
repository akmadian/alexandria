import { beforeEach, describe, expect, it } from "vitest";
import { comboFor, findConflict, normalizeCombo, resetOverrides, resolve, setOverride } from "./keys";

beforeEach(() => {
    localStorage.clear();
    resetOverrides();
});

describe("normalizeCombo", () => {
    it("returns null for bare modifier presses", () => {
        expect(normalizeCombo({ key: "Shift", metaKey: false, ctrlKey: false, shiftKey: true, altKey: false })).toBeNull();
    });

    it("does not add a shift token for printable characters", () => {
        // "5" already encodes the shift state; "shift+5" would be wrong.
        expect(normalizeCombo({ key: "5", metaKey: false, ctrlKey: false, shiftKey: true, altKey: false })).toBe("5");
    });
});

describe("overrides & conflicts", () => {
    it("resolves an action from context + combo", () => {
        expect(resolve("grid", "5")?.id).toBe("rate_5");
        // global actions resolve from any context
        expect(resolve("grid", "mod+k")?.id).toBe("command_palette");
    });

    it("rejects an override that collides with a reachable binding", () => {
        // "5" is rate_5 in grid; assigning it to rate_4 conflicts.
        expect(setOverride("rate_4", "5")).toBe("rate_5");
        expect(comboFor("rate_4")).toBe("4"); // unchanged
    });

    it("accepts a free combo and persists it", () => {
        expect(setOverride("rate_4", "shift+4")).toBeNull();
        expect(comboFor("rate_4")).toBe("shift+4");
        expect(findConflict("rate_4", "shift+4")).toBeNull();
    });
});
