import { afterEach, describe, expect, it, vi } from "vitest";

// The swap point evaluates `"go" in window` at module scope, so each case must
// stub (or not) BEFORE a fresh import of client.ts — and compare against the
// adapter instance from the SAME reset module generation.
afterEach(() => {
    vi.unstubAllGlobals();
    vi.resetModules();
});

describe("client backend selection", () => {
    it("picks the Wails adapter when the bridge is present", async () => {
        vi.stubGlobal("go", {});
        vi.resetModules();
        const { api } = await import("./client");
        const { wailsApi } = await import("./wails-api");
        expect(api).toBe(wailsApi);
    });

    it("keeps the mock without the bridge (bun dev / vitest)", async () => {
        vi.resetModules();
        const { api } = await import("./client");
        const { mockApi } = await import("./mock");
        expect(api).toBe(mockApi);
    });
});
