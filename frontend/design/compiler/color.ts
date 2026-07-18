// OKLCH → sRGB and APCA glue for the validator (design-constitution §23).
// culori's clampChroma mirrors CSS gamut mapping (chroma reduction in OKLCH), so
// the Lc the validator computes is the Lc the webview renders.

import { APCAcontrast, sRGBtoY } from "apca-w3";
import { clampChroma, displayable, rgb } from "culori";

/** The DTCG structured color value used throughout the token source. */
export interface TokenColor {
    colorSpace: string;
    components: [number, number, number];
    alpha?: number;
}

export interface OklchColor {
    l: number;
    c: number;
    h: number;
    alpha: number;
}

export function tokenColor(value: unknown, path: string): OklchColor {
    const candidate = value as TokenColor;
    if (
        candidate === null ||
        typeof candidate !== "object" ||
        candidate.colorSpace !== "oklch" ||
        !Array.isArray(candidate.components)
    ) {
        throw new Error(`token "${path}" is not a structured oklch color`);
    }
    const [l, c, h] = candidate.components;
    return { l, c, h, alpha: candidate.alpha ?? 1 };
}

function toSrgb255(color: OklchColor): [number, number, number] {
    const inGamut = clampChroma({ mode: "oklch", l: color.l, c: color.c, h: color.h }, "oklch");
    const converted = rgb(inGamut);
    if (converted === undefined) throw new Error(`unconvertible color oklch(${color.l} ${color.c} ${color.h})`);
    const channel = (component: number): number => Math.round(Math.min(1, Math.max(0, component)) * 255);
    return [channel(converted.r), channel(converted.g), channel(converted.b)];
}

/** Absolute APCA Lc for text on background. */
export function contrastLc(text: OklchColor, background: OklchColor): number {
    return Math.abs(APCAcontrast(sRGBtoY(toSrgb255(text)), sRGBtoY(toSrgb255(background))));
}

/** True when the color fits the sRGB gamut without chroma clamping. */
export function isDisplayable(color: OklchColor): boolean {
    return displayable({ mode: "oklch", l: color.l, c: color.c, h: color.h });
}

/** Shortest angular distance between two hues, in degrees. */
export function hueDistanceDegrees(first: number, second: number): number {
    const difference = Math.abs(first - second) % 360;
    return difference > 180 ? 360 - difference : difference;
}
