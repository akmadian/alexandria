// Icon — the §14 STRUCTURAL family: every icon is a registered concept (same
// glyph = same meaning, one glyph per concept); components name concepts, never
// glyphs. Icons are ink — they ride currentColor and the interaction states of
// their row, never their own color. The design-source record of this registry is
// registries.json `icons`; the two lists grow together (a compiler emission can
// close the gap when the concept count warrants it). Machinery icons are the
// dot-matrix family (§14), never rows here.
//
// Seeded 2026-07-17 with `disclose` — the first per-need pull from the parked
// token-gaps list (PanelSection's disclosure chevron forced it).

import { ChevronRight, type LucideIcon } from "lucide-react";
import { cx } from "@/lib/cx";
import styles from "./icon.module.css";

export type IconConcept = "disclose";

// C10: a new concept fails to compile until it has exactly one glyph.
const GLYPHS = {
    disclose: ChevronRight,
} as const satisfies Record<IconConcept, LucideIcon>;

export function Icon({ concept, className }: { concept: IconConcept; className?: string }) {
    const Glyph = GLYPHS[concept];
    return <Glyph aria-hidden className={cx(styles.icon, className)} />;
}
