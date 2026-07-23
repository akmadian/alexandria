// Icon — the §14 STRUCTURAL family: every icon is a registered concept (same
// glyph = same meaning, one glyph per concept); components name concepts, never
// glyphs. Icons are ink — they ride currentColor and the interaction states of
// their row, never their own color. The design-source record of this registry is
// registries.json `icons`; the two lists grow together (a compiler emission can
// close the gap when the concept count warrants it). Machinery icons are the
// dot-matrix family (§14), never rows here.
//
// Seeded 2026-07-17 with `disclose` — the first per-need pull from the parked
// token-gaps list (PanelSection's disclosure chevron forced it). `check` and
// `mixed` (§25's em-dash state as a glyph) arrived with Checkbox the same day.
// `settings` (the header gear) joined with the workspace tab strip (task 37).
// The browser-tree concepts (`folder`/`collection`/`tag`/`source`) arrived with
// the Tree primitive (D37) — the §12 rail names a node's kind, one glyph each.
// The keyboard-modifier concepts (`command`/`option`/`control`/`shift`/`return`/
// `delete`) arrived with the Kbd keycap: the Mac symbols aren't in Geist Mono's
// subset (they mush to a blob at 11px via OS fallback), so a shortcut renders them
// as vector icons — crisp at any cap size.

import {
    ArrowBigUp,
    Check,
    ChevronRight,
    ChevronUp,
    Command,
    CornerDownLeft,
    Delete,
    Flag,
    FlagOff,
    Folder,
    HardDrive,
    Layers,
    Minus,
    Option,
    Settings,
    Star,
    Tag,
    type LucideIcon,
} from "lucide-react";
import { cx } from "@/lib/cx";
import styles from "./icon.module.css";

export type IconConcept =
    | "check"
    | "disclose"
    | "mixed"
    | "rating"
    | "flag"
    | "reject"
    | "settings"
    | "folder"
    | "collection"
    | "tag"
    | "source"
    | "command"
    | "option"
    | "control"
    | "shift"
    | "return"
    | "delete";

// C10: a new concept fails to compile until it has exactly one glyph. Judgment
// concepts (rating/flag/reject) seeded with the §19 cell slots — icons are ink,
// fill = on (§14): a rated star / picked flag renders filled, unrated/unflagged
// renders nothing (the slot is silent, §10).
const GLYPHS = {
    check: Check,
    disclose: ChevronRight,
    mixed: Minus,
    rating: Star,
    flag: Flag,
    reject: FlagOff,
    settings: Settings,
    folder: Folder,
    collection: Layers,
    tag: Tag,
    source: HardDrive,
    command: Command,
    option: Option,
    control: ChevronUp,
    shift: ArrowBigUp,
    return: CornerDownLeft,
    delete: Delete,
} as const satisfies Record<IconConcept, LucideIcon>;

export function Icon({ concept, className }: { concept: IconConcept; className?: string }) {
    const Glyph = GLYPHS[concept];
    return <Glyph aria-hidden className={cx(styles.icon, className)} />;
}
