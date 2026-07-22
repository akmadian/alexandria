// The §23 validator: executes contracts.json + the registries' structural rules
// against resolved theme trees. Failures block emission (main.ts exits nonzero);
// warnings surface drift without blocking (register-step multiples predate their
// own tokenization — see the dated note in the round's decision entry).

import { contrastLc, hueDistanceDegrees, isDisplayable, tokenColor, type OklchColor } from "./color";
import type { ResolvedSource, ResolvedToken, ThemeTokens } from "./resolve";

export interface ContrastPair {
    ink: string;
    on: string | string[];
    minLc: number;
    /** Dark-polarity override (adjudicated 2026-07-17) — absent = one target for both. */
    minLcDark?: number;
}

export interface ContractsDocument {
    text: { pairs: ContrastPair[] };
    separation: {
        deltaLBand: [number, number];
        families: Record<string, { stepOrder: string[] }>;
        hairline: { pair: [string, string]; deltaLBand: [number, number] };
    };
    ring: { pairs: { ink: string; on: string; minLc: number }[] };
    "selected-text": { pairs: ContrastPair[] };
    tag: { pairs: { ink: string; on: string; minLc: number }[] };
    minHueDistanceDeg: { value: number };
}

export interface RegistriesDocument {
    familyDirection: { resolved: Record<string, Record<string, "recess" | "raise">> };
    tagRecipes: { hues: string[] };
    hueEligibility: {
        accent: { exclude: string[]; default: string };
    };
}

export interface ValidationResult {
    failures: string[];
    warnings: string[];
    /** Hues passing the ring contract on every theme (the picker's offer list). */
    accentEligible: string[];
}

/** Maps contracts.json family names onto familyDirection's keys. */
const FAMILY_DIRECTION_KEY: Record<string, string> = { surface: "chrome", cell: "cell" };

const QUANTUM_PX = 4;
const QUANTUM_EXEMPT_PATHS = new Set(["size.icon-stroke", "size.control-inset"]);
const DEAD_BAND: [number, number] = [0.45, 0.65];
const REGISTER_STEP_TOLERANCE = 0.0015;

export function validate(
    source: ResolvedSource,
    contracts: ContractsDocument,
    registries: RegistriesDocument,
): ValidationResult {
    const failures: string[] = [];
    const warnings: string[] = [];

    const lookup = (tokens: ThemeTokens, path: string, themeName: string, context: string): ResolvedToken | undefined => {
        const token = tokens.get(path);
        if (token === undefined) failures.push(`${themeName}: token "${path}" required by ${context} does not exist`);
        return token;
    };

    const colorAt = (tokens: ThemeTokens, path: string, themeName: string, context: string): OklchColor | undefined => {
        const token = lookup(tokens, path, themeName, context);
        if (token === undefined) return undefined;
        try {
            return tokenColor(token.value, path);
        } catch (err) {
            failures.push(`${themeName}: ${(err as Error).message} (${context})`);
            return undefined;
        }
    };

    const round = (value: number): number => Math.round(value * 10000) / 10000;

    // A theme is dark ⇔ its chrome family raises (§7: interaction moves toward ink —
    // raising means the ink is light, i.e. dark polarity). Derived, never re-declared.
    const isDarkTheme = (themeName: string): boolean =>
        registries.familyDirection.resolved["chrome"]?.[themeName] === "raise";

    // -- Text contrast (text.pairs + selected-text.pairs), per theme and polarity.
    // A pair whose surface is under stage.* is theme-independent, checked across the range.
    const checkContrastPairs = (pairs: ContrastPair[], checkName: string): void => {
        for (const pair of pairs) {
            const surfaces = Array.isArray(pair.on) ? pair.on : [pair.on];
            for (const [themeName, tokens] of source.themes) {
                const requiredLc = isDarkTheme(themeName) ? (pair.minLcDark ?? pair.minLc) : pair.minLc;
                for (const surfacePath of surfaces) {
                    if (surfacePath.startsWith("stage")) continue; // handled below, once
                    const ink = colorAt(tokens, pair.ink, themeName, checkName);
                    const surface = colorAt(tokens, surfacePath, themeName, checkName);
                    if (ink === undefined || surface === undefined) continue;
                    const measured = contrastLc(ink, surface);
                    if (measured < requiredLc) {
                        failures.push(
                            `${themeName}: ${pair.ink} on ${surfacePath} — Lc ${round(measured)} < ${requiredLc} (${checkName})`,
                        );
                    }
                }
            }
            // Stage pairs: theme-independent, must hold across the adjustable range
            // (§23). The stage is dark by construction (§1) — dark targets apply.
            const stageSurfaces = surfaces.filter((surfacePath) => surfacePath.startsWith("stage"));
            if (stageSurfaces.length > 0) {
                const tokens = source.themes.get(source.defaultTheme);
                if (tokens === undefined) continue;
                const requiredLc = pair.minLcDark ?? pair.minLc;
                for (const bound of ["stage.min", "stage.default", "stage.max"]) {
                    const ink = colorAt(tokens, pair.ink, "stage", checkName);
                    const stage = colorAt(tokens, bound, "stage", checkName);
                    if (ink === undefined || stage === undefined) continue;
                    const measured = contrastLc(ink, stage);
                    if (measured < requiredLc) {
                        failures.push(`stage: ${pair.ink} on ${bound} — Lc ${round(measured)} < ${requiredLc} (${checkName})`);
                    }
                }
            }
        }
    };
    checkContrastPairs(contracts.text.pairs, "text");
    checkContrastPairs(contracts["selected-text"].pairs, "selected-text");

    // -- Register separation: every adjacent step in band, monotonic in the declared
    // direction, and (warning-grade) a whole multiple of the world's register step.
    const [deltaFloor, deltaCap] = contracts.separation.deltaLBand;
    for (const [familyName, family] of Object.entries(contracts.separation.families)) {
        const directionKey = FAMILY_DIRECTION_KEY[familyName] ?? familyName;
        for (const [themeName, tokens] of source.themes) {
            const direction = registries.familyDirection.resolved[directionKey]?.[themeName];
            if (direction === undefined) {
                failures.push(`${themeName}: no familyDirection row for "${directionKey}"`);
                continue;
            }
            const registerStepToken = tokens.get("world.register-step");
            const registerStep = typeof registerStepToken?.value === "number" ? registerStepToken.value : undefined;
            const steps = family.stepOrder.map((stepName) => ({
                path: `${familyName}.${stepName}`,
                color: colorAt(tokens, `${familyName}.${stepName}`, themeName, "separation"),
            }));
            for (let i = 0; i < steps.length - 1; i++) {
                const current = steps[i].color;
                const next = steps[i + 1].color;
                if (current === undefined || next === undefined) continue;
                const delta = next.l - current.l;
                const towardInk = direction === "recess" ? delta < 0 : delta > 0;
                if (!towardInk) {
                    failures.push(
                        `${themeName}: ${steps[i].path} → ${steps[i + 1].path} moves against the family direction "${direction}" (ΔL ${round(delta)})`,
                    );
                }
                const magnitude = Math.abs(delta);
                if (magnitude < deltaFloor || magnitude > deltaCap) {
                    failures.push(
                        `${themeName}: ${steps[i].path} → ${steps[i + 1].path} — ΔL ${round(magnitude)} outside [${deltaFloor}, ${deltaCap}] (separation)`,
                    );
                }
                if (registerStep !== undefined) {
                    const multiples = magnitude / registerStep;
                    if (Math.abs(multiples - Math.round(multiples)) * registerStep > REGISTER_STEP_TOLERANCE) {
                        warnings.push(
                            `${themeName}: ${steps[i].path} → ${steps[i + 1].path} — ΔL ${round(magnitude)} is not a register-step multiple (step ${registerStep})`,
                        );
                    }
                }
            }
        }
    }

    // -- Hairline band.
    {
        const [hairlinePath, surfacePath] = contracts.separation.hairline.pair;
        const [floor, cap] = contracts.separation.hairline.deltaLBand;
        for (const [themeName, tokens] of source.themes) {
            const hairline = colorAt(tokens, hairlinePath, themeName, "hairline");
            const surface = colorAt(tokens, surfacePath, themeName, "hairline");
            if (hairline === undefined || surface === undefined) continue;
            const delta = Math.abs(hairline.l - surface.l);
            if (delta < floor || delta > cap) {
                failures.push(`${themeName}: hairline ΔL ${round(delta)} outside [${floor}, ${cap}]`);
            }
        }
    }

    // -- Structure: quantum membership for space.* and size.* dimensions.
    {
        const tokens = source.themes.get(source.defaultTheme);
        if (tokens !== undefined) {
            for (const token of tokens.values()) {
                const inScope = token.path.startsWith("space.") || token.path.startsWith("size.");
                if (!inScope || QUANTUM_EXEMPT_PATHS.has(token.path) || token.type !== "dimension") continue;
                const dimension = token.value as { value: number; unit: string };
                if (dimension.value % QUANTUM_PX !== 0) {
                    failures.push(
                        `${token.path} — ${dimension.value}${dimension.unit} is not a ${QUANTUM_PX}px quantum multiple`,
                    );
                }
            }
        }
    }

    // -- Structure: no theme surface in the dead band; chrome is achromatic.
    for (const [themeName, tokens] of source.themes) {
        for (const token of tokens.values()) {
            const isSurface = token.path.startsWith("surface.") || token.path.startsWith("cell.");
            const isChromeColor = isSurface || token.path.startsWith("ink.") || token.path.startsWith("stage.");
            if (!isChromeColor || token.type !== "color") continue;
            const color = tokenColor(token.value, token.path);
            if (isSurface && color.l > DEAD_BAND[0] && color.l < DEAD_BAND[1]) {
                failures.push(`${themeName}: ${token.path} — L ${color.l} sits in the dead band (${DEAD_BAND[0]}–${DEAD_BAND[1]})`);
            }
            if (color.c !== 0) {
                failures.push(`${themeName}: ${token.path} — chroma ${color.c} on chrome (must be exactly 0, §1)`);
            }
        }
    }

    // -- Attention hue distance from every enabled label hue.
    {
        const tokens = source.themes.get(source.defaultTheme);
        if (tokens !== undefined) {
            const attention = colorAt(tokens, "attention", source.defaultTheme, "hue-distance");
            if (attention !== undefined) {
                for (const token of tokens.values()) {
                    if (!token.path.startsWith("label.")) continue;
                    const label = tokenColor(token.value, token.path);
                    const distance = hueDistanceDegrees(attention.h, label.h);
                    if (distance < contracts.minHueDistanceDeg.value) {
                        failures.push(
                            `attention vs ${token.path} — hue distance ${round(distance)}° < ${contracts.minHueDistanceDeg.value}°`,
                        );
                    }
                }
            }
        }
    }

    // -- Ring gate: the accent-eligibility table. The DEFAULT hue failing anywhere is
    // fatal (it must be offerable); any other hue failing is excluded + warned.
    const accentEligible: string[] = [];
    {
        const ringRule = contracts.ring.pairs[0];
        const excluded = new Set(registries.hueEligibility.accent.exclude);
        const candidateHues = registries.tagRecipes.hues.filter((hue) => !excluded.has(hue));
        for (const hue of candidateHues) {
            const failedThemes: string[] = [];
            for (const [themeName, tokens] of source.themes) {
                const ringInk = colorAt(tokens, ringRule.ink.replace("<hue>", hue), themeName, "ring");
                const panel = colorAt(tokens, ringRule.on, themeName, "ring");
                if (ringInk === undefined || panel === undefined) continue;
                const measured = contrastLc(ringInk, panel);
                if (measured < ringRule.minLc) failedThemes.push(`${themeName} (Lc ${round(measured)})`);
            }
            if (failedThemes.length === 0) {
                accentEligible.push(hue);
            } else if (hue === registries.hueEligibility.accent.default) {
                failures.push(`ring: default accent "${hue}" fails on ${failedThemes.join(", ")} — the default must be offerable`);
            } else {
                warnings.push(`ring: "${hue}" not accent-eligible — fails on ${failedThemes.join(", ")}`);
            }
        }
    }

    // -- Tag chip legibility, per hue × per theme (covers both worlds).
    for (const pairTemplate of contracts.tag.pairs) {
        for (const hue of registries.tagRecipes.hues) {
            const inkPath = pairTemplate.ink.replace("<hue>", hue);
            const surfacePath = pairTemplate.on.replace("<hue>", hue);
            for (const [themeName, tokens] of source.themes) {
                const ink = colorAt(tokens, inkPath, themeName, "tag");
                const surface = colorAt(tokens, surfacePath, themeName, "tag");
                if (ink === undefined || surface === undefined) continue;
                const measured = contrastLc(ink, surface);
                if (measured < pairTemplate.minLc) {
                    failures.push(`${themeName}: ${inkPath} on ${surfacePath} — Lc ${round(measured)} < ${pairTemplate.minLc} (tag)`);
                }
            }
        }
    }

    // -- Gamut (warning): a color needing chroma clamping renders slightly off-spec.
    // Summarized to one line — per-token noise drowns the real findings; §28 already
    // tracks P3 headroom as a known refinement.
    {
        const outOfGamut = new Set<string>();
        for (const tokens of source.themes.values()) {
            for (const token of tokens.values()) {
                if (token.type !== "color") continue;
                let color: OklchColor;
                try {
                    color = tokenColor(token.value, token.path);
                } catch {
                    continue; // non-structured color values already failed elsewhere
                }
                if (color.c === 0) continue;
                if (!isDisplayable(color)) outOfGamut.add(token.path);
            }
        }
        if (outOfGamut.size > 0) {
            warnings.push(
                `${outOfGamut.size} chromatic token path(s) exceed the sRGB gamut and chroma-clamp at render (§28 P3 headroom): ${[...outOfGamut].sort().join(", ")}`,
            );
        }
    }

    return { failures: dedupe(failures), warnings: dedupe(warnings), accentEligible };
}

/** Theme-invariant findings repeat per theme loop; report each once. */
function dedupe(messages: string[]): string[] {
    return [...new Set(messages)];
}
