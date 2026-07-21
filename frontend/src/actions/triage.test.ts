// The grid-context triage keymap + registry (task 34). The dispatch is the pure
// (key → action → absolute patch) resolution; these pin the LrC grammar
// (0–5 rate, 6–9 label, − clears, P/X/U flag) and the absolute-value contract.

import { describe, expect, it } from "vitest";
import { resolveGridTriageAction, triageActions } from "./triage";

describe("resolveGridTriageAction (grid keymap)", () => {
    it("maps 0–5 to rate verbs, 0 clearing (rating = null)", () => {
        expect(resolveGridTriageAction("0")?.patch).toEqual({ rating: null });
        expect(resolveGridTriageAction("1")?.patch).toEqual({ rating: 1 });
        expect(resolveGridTriageAction("3")?.patch).toEqual({ rating: 3 });
        expect(resolveGridTriageAction("5")?.patch).toEqual({ rating: 5 });
    });

    it("maps 6–9 to labels and − to clear (colorLabel = null)", () => {
        expect(resolveGridTriageAction("6")?.patch).toEqual({ colorLabel: "red" });
        expect(resolveGridTriageAction("7")?.patch).toEqual({ colorLabel: "yellow" });
        expect(resolveGridTriageAction("8")?.patch).toEqual({ colorLabel: "green" });
        expect(resolveGridTriageAction("9")?.patch).toEqual({ colorLabel: "blue" });
        expect(resolveGridTriageAction("-")?.patch).toEqual({ colorLabel: null });
    });

    it("maps P pick / X reject / U clear, case-insensitively", () => {
        expect(resolveGridTriageAction("p")?.patch).toEqual({ flag: "pick" });
        expect(resolveGridTriageAction("P")?.patch).toEqual({ flag: "pick" }); // shift form
        expect(resolveGridTriageAction("x")?.patch).toEqual({ flag: "reject" });
        expect(resolveGridTriageAction("u")?.patch).toEqual({ flag: null });
    });

    it("returns null for a key with no triage verb (falls through to navigation)", () => {
        expect(resolveGridTriageAction("j")).toBeNull();
        expect(resolveGridTriageAction("ArrowDown")).toBeNull();
        expect(resolveGridTriageAction(" ")).toBeNull();
    });
});

describe("triage registry", () => {
    it("keys every entry by its own id (the completeness-trick invariant)", () => {
        for (const [key, action] of Object.entries(triageActions)) {
            expect(action.id).toBe(key);
        }
    });

    it("carries absolute values, never deltas", () => {
        // A rate verb fully specifies the rating; there is no +1/−1 in the vocabulary.
        expect(triageActions["rate-4"].patch).toEqual({ rating: 4 });
        expect(triageActions["flag-clear"].patch).toEqual({ flag: null });
    });
});
