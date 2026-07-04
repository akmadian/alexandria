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
