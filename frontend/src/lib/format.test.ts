// The inspector's data-voice formatters (§9 mono values): photographic
// notation is exact, locale-aware where Intl owns it.

import { describe, expect, it } from "vitest";
import { formatAperture, formatExposure, formatFocalLength, formatGps } from "./format";

describe("formatAperture", () => {
    it("keeps one decimal and drops trailing zeros", () => {
        expect(formatAperture(3.2)).toBe("ƒ/3.2");
        expect(formatAperture(8)).toBe("ƒ/8");
        expect(formatAperture(1.8)).toBe("ƒ/1.8");
    });
});

describe("formatExposure", () => {
    it("composes both halves through the localized template", () => {
        expect(formatExposure("1/80", 3.2)).toBe("1/80 at ƒ/3.2");
    });
    it("renders a lone half without the template", () => {
        expect(formatExposure("1/80", null)).toBe("1/80");
        expect(formatExposure(null, 8)).toBe("ƒ/8");
    });
    it("returns null when neither half exists (the row is absent)", () => {
        expect(formatExposure(null, null)).toBeNull();
    });
});

describe("formatFocalLength", () => {
    it("renders millimeters as a unit", () => {
        expect(formatFocalLength(50)).toBe("50 mm");
        expect(formatFocalLength(18.5)).toBe("18.5 mm");
    });
});

describe("formatGps", () => {
    it("splits hemispheres and keeps absolute degrees", () => {
        expect(formatGps(47.6062, -122.3321)).toBe("47.6062° N, 122.3321° W");
        expect(formatGps(-33.8688, 151.2093)).toBe("33.8688° S, 151.2093° E");
    });
});
