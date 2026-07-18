// Ambient types for apca-w3 (plain JS, no bundled declarations). Only the two
// functions the validator uses.
declare module "apca-w3" {
    /** Luminance Y from an [r, g, b] triple in 0–255. */
    export function sRGBtoY(rgb: [number, number, number]): number;
    /** Signed Lc contrast (text Y, background Y); positive = dark on light. */
    export function APCAcontrast(textY: number, backgroundY: number): number;
}
