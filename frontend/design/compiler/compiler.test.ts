// Compiler tests (task-23 acceptance): broken fixtures fail with named violations;
// the real source resolves cleanly; emission has the ratified shapes. The
// real-source CONTRACT verdict is pinned to the documented adjudication set — a
// new failure class appearing is a regression this file catches.

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, test } from "vitest";
import { contrastLc, hueDistanceDegrees, tokenColor, type TokenColor } from "./color";
import { emit } from "./emit";
import { loadSource, resolveSource, resolveTheme, type RawNode, type ResolvedSource } from "./resolve";
import { validate, type ContractsDocument, type RegistriesDocument } from "./validate";

const designDirectory = join(dirname(fileURLToPath(import.meta.url)), "..");
const realSource = resolveSource(loadSource(join(designDirectory, "tokens.resolver.json")));
const contracts = JSON.parse(readFileSync(join(designDirectory, "contracts.json"), "utf8")) as ContractsDocument;
const registries = JSON.parse(readFileSync(join(designDirectory, "registries.json"), "utf8")) as RegistriesDocument;

function cloneSource(source: ResolvedSource): ResolvedSource {
    return {
        themeNames: [...source.themeNames],
        defaultTheme: source.defaultTheme,
        themes: new Map(
            [...source.themes].map(([themeName, tokens]) => [
                themeName,
                new Map([...tokens].map(([path, token]) => [path, structuredClone(token)])),
            ]),
        ),
    };
}

function setComponent(source: ResolvedSource, themeName: string, path: string, index: 0 | 1 | 2, value: number): void {
    const token = source.themes.get(themeName)?.get(path);
    if (token === undefined) throw new Error(`fixture: no token ${themeName}/${path}`);
    (token.value as TokenColor).components[index] = value;
}

describe("resolve", () => {
    test("real source: four themes, same token count, no broken references", () => {
        expect(realSource.themeNames).toEqual(["paper", "linen", "graphite", "carbon"]);
        expect(realSource.defaultTheme).toBe("paper");
        const sizes = realSource.themeNames.map((themeName) => realSource.themes.get(themeName)?.size);
        expect(new Set(sizes).size).toBe(1);
        expect(sizes[0]).toBeGreaterThan(150);
    });

    test("aliases chase to structured values and record their source", () => {
        const accent = realSource.themes.get("paper")?.get("accent");
        expect(accent?.aliasOf).toBe("color.blue.solid");
        expect(tokenColor(accent?.value, "accent").h).toBe(252);
        expect(accent?.varying).toBe(false);
    });

    test("world layers vary per theme and mark tokens varying", () => {
        const lightStep = realSource.themes.get("paper")?.get("world.register-step");
        const darkStep = realSource.themes.get("carbon")?.get("world.register-step");
        expect(lightStep?.value).toBe(0.018);
        expect(darkStep?.value).toBe(0.035);
        expect(darkStep?.varying).toBe(true);
        expect(realSource.themes.get("graphite")?.get("color.blue.tint")?.varying).toBe(true);
    });

    test("typography composites resolve nested aliases under an inherited $type", () => {
        const control = realSource.themes.get("paper")?.get("type-role.control");
        expect(control?.type).toBe("typography");
        const value = control?.value as { fontFamily: string[]; fontWeight: number };
        expect(value.fontFamily[0]).toBe("Geist");
        expect(value.fontWeight).toBe(500);
    });

    const layer = (document: RawNode, modifier = false) => ({ document, modifier });

    test("a broken alias names itself and its requirer", () => {
        const document: RawNode = {
            color: { $type: "color", real: { $value: "{color.ghost}" } },
        };
        expect(() => resolveTheme([layer(document)], "test")).toThrow(/"\{color\.ghost\}" referenced by "color\.real"/);
    });

    test("an alias cycle is named, not an overflow", () => {
        const document: RawNode = {
            first: { $type: "color", $value: "{second}" },
            second: { $type: "color", $value: "{first}" },
        };
        expect(() => resolveTheme([layer(document)], "test")).toThrow(/alias cycle/);
    });

    test("a later modifier layer wins and marks the token varying", () => {
        const base: RawNode = { ink: { $type: "color", main: { $value: "base" } } };
        const override: RawNode = { ink: { main: { $value: "themed" } } };
        const resolved = resolveTheme([layer(base), layer(override, true)], "test");
        expect(resolved.get("ink.main")?.value).toBe("themed");
        expect(resolved.get("ink.main")?.varying).toBe(true);
    });

    test("a layer re-typing a token is a named failure — overrides re-value, never re-type", () => {
        const base: RawNode = { size: { $type: "dimension", control: { $value: 24 } } };
        const override: RawNode = { size: { control: { $type: "number", $value: 24 } } };
        expect(() => resolveTheme([layer(base), layer(override, true)], "test")).toThrow(
            /"size\.control" re-typed across layers \("dimension" → "number"\)/,
        );
    });
});

describe("validate — broken fixtures fail with named violations", () => {
    test("an ink losing its APCA contract is named with theme, pair, and Lc", () => {
        const broken = cloneSource(realSource);
        setComponent(broken, "paper", "ink.1", 0, 0.9);
        const result = validate(broken, contracts, registries);
        expect(result.failures.join("\n")).toMatch(/paper: ink\.1 on surface\.panel — Lc [\d.]+ < 75 \(text\)/);
    });

    test("a register step below the ΔL floor fails separation", () => {
        const broken = cloneSource(realSource);
        setComponent(broken, "paper", "surface.hover", 0, 0.974);
        const result = validate(broken, contracts, registries);
        expect(result.failures.join("\n")).toMatch(/paper: surface\.panel → surface\.hover — ΔL 0\.001 outside/);
    });

    test("a step moving against the family direction fails monotonicity", () => {
        const broken = cloneSource(realSource);
        setComponent(broken, "paper", "surface.hover", 0, 0.99);
        const result = validate(broken, contracts, registries);
        expect(result.failures.join("\n")).toMatch(/moves against the family direction "recess"/);
    });

    test("an off-quantum size is rejected", () => {
        const broken = cloneSource(realSource);
        const token = broken.themes.get("paper")?.get("size.control");
        (token?.value as { value: number }).value = 13;
        const result = validate(broken, contracts, registries);
        expect(result.failures.join("\n")).toMatch(/size\.control — 13px is not a 4px quantum multiple/);
    });

    test("a surface in the dead band is rejected", () => {
        const broken = cloneSource(realSource);
        setComponent(broken, "graphite", "surface.panel", 0, 0.5);
        const result = validate(broken, contracts, registries);
        expect(result.failures.join("\n")).toMatch(/graphite: surface\.panel — L 0\.5 sits in the dead band/);
    });

    test("chroma on chrome is rejected exactly", () => {
        const broken = cloneSource(realSource);
        setComponent(broken, "paper", "surface.panel", 1, 0.004);
        const result = validate(broken, contracts, registries);
        expect(result.failures.join("\n")).toMatch(/paper: surface\.panel — chroma 0\.004 on chrome/);
    });

    test("an attention hue crowding a label hue fails the distance floor", () => {
        const broken = cloneSource(realSource);
        setComponent(broken, "paper", "attention", 2, 320);
        const result = validate(broken, contracts, registries);
        expect(result.failures.join("\n")).toMatch(/attention vs label\.purple — hue distance 15° < 30°/);
    });

    test("a missing required token is a failure, not a crash", () => {
        const broken = cloneSource(realSource);
        broken.themes.get("paper")?.delete("ink.1");
        const result = validate(broken, contracts, registries);
        expect(result.failures.join("\n")).toMatch(/paper: token "ink\.1" required by text does not exist/);
    });
});

describe("validate — the real source", () => {
    const verdict = validate(realSource, contracts, registries);

    test("every contract holds — the palette round, machine-certified (adjudicated 2026-07-17)", () => {
        expect(verdict.failures).toEqual([]);
    });

    test("the default accent is eligible; gray never is; eligibility ⊆ the palette", () => {
        expect(verdict.accentEligible).toContain(registries.hueEligibility.accent.default);
        expect(verdict.accentEligible).not.toContain("gray");
        for (const hue of verdict.accentEligible) expect(registries.tagRecipes.hues).toContain(hue);
    });
});

describe("emit", () => {
    const files = emit(realSource, ["blue"]);

    test(":root carries the complete default theme", () => {
        expect(files.css).toContain(`--alx-surface-panel: oklch(0.975 0 0);`);
        expect(files.css).toContain(`--alx-space-3: 12px;`);
        expect(files.css).toContain(`--alx-radius-control: 4px;`);
        expect(files.css).toContain(`--alx-duration-fast: 80ms;`);
        expect(files.css).toContain(`--alx-easing-out: cubic-bezier(0.16, 1, 0.3, 1);`);
        expect(files.css).toContain(`--alx-font-mono: "Geist Mono", ui-monospace, monospace;`);
    });

    test("non-default themes override only their varying subset", () => {
        const carbonBlock = files.css.split(`[data-theme="carbon"]`)[1]?.split("}")[0] ?? "";
        expect(carbonBlock).toContain(`--alx-surface-panel: oklch(0.215 0 0);`);
        expect(carbonBlock).toContain(`--alx-color-blue-tint:`);
        expect(carbonBlock).not.toContain(`--alx-space-3`);
        expect(carbonBlock).not.toContain(`--alx-radius-control`);
    });

    test("shadows compose with the house inset extension", () => {
        expect(files.css).toContain(
            `--alx-shadow-occlusion: 0px 4px 16px 0px oklch(0 0 0 / 0.16), 0px 1px 3px 0px oklch(0 0 0 / 0.1);`,
        );
        expect(files.css).toContain(`--alx-shadow-tunnel: inset 0px 6px 8px -6px oklch(0 0 0 / 0.25);`);
        expect(files.css).toContain(`inset 0px 1px 0px 0px oklch(1 0 0 / 0.3), inset 0px 0px 0px 1px oklch(1 0 0 / 0.14)`);
    });

    test("the fun gradient composes with its angle token", () => {
        expect(files.css).toContain(`--alx-fun-gradient: linear-gradient(100deg, oklch(0.72 0.16 15) 0%,`);
    });

    test("type roles emit per-property variables plus the unit class with paired ink", () => {
        expect(files.css).toContain(`--alx-type-role-control-font-size: 12px;`);
        expect(files.css).toContain(`.alx-type-control {`);
        expect(files.css).toContain(`font-size: var(--alx-type-role-control-font-size);`);
        expect(files.css).toMatch(/\.alx-type-label-sm \{[^}]*color: var\(--alx-ink-3\);/s);
        expect(files.css).toMatch(/\.alx-type-data \{[^}]*font-variant-numeric: tabular-nums;/s);
    });

    test("type-scale composites are not emitted — roles supersede them", () => {
        expect(files.css).not.toContain(`--alx-type-scale`);
    });

    test("accent and attention carry their bound hue's on-solid ink and ring step", () => {
        expect(files.css).toContain(`--alx-accent-on: oklch(1 0 0);`);
        expect(files.css).toContain(`--alx-attention-on: oklch(1 0 0);`);
        expect(files.css).toContain(`--alx-accent-ring: oklch(0.56 0.195 252);`);
        const carbonBlock = files.css.split(`[data-theme="carbon"]`)[1]?.split("}")[0] ?? "";
        expect(carbonBlock).toContain(`--alx-accent-ring: oklch(0.74 0.12 252);`);
    });

    test("tokens.ts derives the theme vocabulary from the resolver", () => {
        expect(files.typescript).toContain(`export const themes = ["paper", "linen", "graphite", "carbon"] as const;`);
        expect(files.typescript).toContain(`export const defaultTheme: Theme = "paper";`);
    });

    test("the reference table is parseable and theme-aware", () => {
        const reference = JSON.parse(files.referenceJson) as {
            accentEligible: string[];
            tokens: { path: string; varying: boolean; css?: unknown }[];
        };
        expect(reference.accentEligible).toEqual(["blue"]);
        const panel = reference.tokens.find((entry) => entry.path === "surface.panel");
        expect(panel?.varying).toBe(true);
        expect((panel?.css as Record<string, string>).carbon).toBe("oklch(0.215 0 0)");
        const space = reference.tokens.find((entry) => entry.path === "space.1");
        expect(space?.css).toBe("4px");
    });
});

describe("color", () => {
    test("hue distance is circular", () => {
        expect(hueDistanceDegrees(345, 25)).toBe(40);
        expect(hueDistanceDegrees(10, 350)).toBe(20);
    });

    test("contrast is APCA-shaped", () => {
        const black = { l: 0, c: 0, h: 0, alpha: 1 };
        const white = { l: 1, c: 0, h: 0, alpha: 1 };
        expect(contrastLc(black, white)).toBeGreaterThan(100);
        expect(contrastLc(white, white)).toBe(0);
    });

    test("non-structured colors are rejected by name", () => {
        expect(() => tokenColor("#fff", "some.path")).toThrow(/"some\.path" is not a structured oklch color/);
    });
});
