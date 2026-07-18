import "@testing-library/jest-dom/vitest";
import "@/i18n"; // real English catalog so t() returns readable strings in tests

// Node 26 ships a disabled global `localStorage` and happy-dom 20 doesn't expose
// one on its window, so app code using bare `localStorage` (theme, keybindings,
// tree/pane persistence) has nothing to talk to. A Map-backed Storage covers it
// for tests — smaller and more predictable than wiring --localstorage-file.
class MemoryStorage implements Storage {
    private m = new Map<string, string>();
    get length() {
        return this.m.size;
    }
    clear() {
        this.m.clear();
    }
    getItem(k: string) {
        return this.m.get(k) ?? null;
    }
    setItem(k: string, v: string) {
        this.m.set(k, String(v));
    }
    removeItem(k: string) {
        this.m.delete(k);
    }
    key(i: number) {
        return [...this.m.keys()][i] ?? null;
    }
}
Object.defineProperty(globalThis, "localStorage", { value: new MemoryStorage(), configurable: true, writable: true });

// happy-dom has no layout engine: every element measures 0×0, which starves the
// virtualized grid (zero columns, zero visible rows). Tests get a fixed 800×600
// viewport — constants beat universal zeros. The stubs live on
// HTMLElement.prototype because that's where happy-dom defines the metrics
// (an Element.prototype stub is shadowed by the subclass and never read).
// happy-dom's own ResizeObserver fires 0×0 entries that would overwrite the
// stubbed rects (tanstack-virtual trusts them), so it becomes a no-op too.
class NoopResizeObserver implements ResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
}
globalThis.ResizeObserver = NoopResizeObserver;
window.ResizeObserver = NoopResizeObserver; // tanstack-virtual reads it off targetWindow
for (const [metric, value] of [
    ["clientWidth", 800],
    ["clientHeight", 600],
    ["offsetWidth", 800], // tanstack-virtual's getRect reads offset*, not client*
    ["offsetHeight", 600],
] as const) {
    Object.defineProperty(HTMLElement.prototype, metric, { configurable: true, get: () => value });
}
HTMLElement.prototype.getBoundingClientRect = () =>
    ({ x: 0, y: 0, top: 0, left: 0, right: 800, bottom: 600, width: 800, height: 600, toJSON: () => "" }) as DOMRect;
