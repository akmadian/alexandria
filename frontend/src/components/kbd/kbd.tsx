// Kbd — the keyboard-shortcut keycap (the reference set's ⌘F chips). A domain-blind
// presentational primitive: a native <kbd> carrying one key glyph/label, in one of two styles.
// Multi-key combos compose via KbdGroup (one <kbd> per key) — the shadcn model. Kbd stays
// ATOMIC so the group owns the row + gap; no prop-per-combination (§22).
//
// Two styles behind a `style` prop, mirroring Badge:
//   flat   — quiet tinted box, no border/shadow (the machinery-chip default: fill XOR border, D32).
//   keycap — a bordered face with a heavier bottom rule, the pressable-key read. Leans on §6's
//            genre carve-out ("keyboard hints — labels, not surfaces"); the lift is a BORDER,
//            never a box-shadow, so docked chrome stays flat (§6).
//
// Neutral by design — chrome is hue-free, so no `hue` prop. Sizes ride the D33 control-size bundle
// (a keycap is control-like): `xs` (16, dense) · `sm` (20, the menu default) · `md` (24), each a
// tier of {mono text role + height + icon size} derived together. text-box-trim (see kbd.module.css)
// optically centers the mono glyph AND frees the xs tier from the 16px line-height. The ladder stops
// at md because mono ceilings at 12px (a taller cap needs a design-source mono-ramp extension).
//
// Modifier keys (⌘⇧⌥⌃⌫↵) render via `icon`, not a glyph: the Mac symbols aren't in Geist Mono's
// subset, so at 11px they mush to a blob through OS fallback. As registered icon concepts they're
// vector — crisp at any cap size. `.kbd` sizes them via `--alx-size-icon` (D33 sized container).

import type { ReactNode } from "react";
import { Icon, type IconConcept } from "@/components/icon/icon";
import { cx } from "@/lib/cx";
import styles from "./kbd.module.css";

export type KbdStyle = "flat" | "keycap";

export type KbdSize = "xs" | "sm" | "md";

// C10: a new style/size fails to compile until it has a class.
const STYLE_CLASSES = {
    flat: styles.flat,
    keycap: styles.keycap,
} as const satisfies Record<KbdStyle, string>;

const SIZE_CLASSES = {
    xs: styles.xs,
    sm: styles.sm,
    md: styles.md,
} as const satisfies Record<KbdSize, string>;

export function Kbd({
    children,
    icon,
    style = "flat",
    size = "sm",
}: {
    /** Text key (letter/word: `E`, `Esc`). Omit when `icon` is set. */
    children?: ReactNode;
    /** A modifier-key icon concept (`command`, `shift`, …) — the vector path for glyphs that
     * mush as font text. Sugar mirroring Menu's `icon`; renders in place of `children`. */
    icon?: IconConcept;
    style?: KbdStyle;
    /** Cap size on the control height ramp: `xs` 16 (dense) · `sm` 20 (menu) · `md` 24. */
    size?: KbdSize;
}) {
    return (
        <kbd className={cx(styles.kbd, STYLE_CLASSES[style], SIZE_CLASSES[size])}>
            {icon ? <Icon concept={icon} /> : children}
        </kbd>
    );
}

// KbdGroup — the multi-key row (⌘ ⇧ P): composes Kbd children (and plain separators like `+`)
// into one flex line so the caps stay evenly spaced.
export function KbdGroup({ children }: { children: ReactNode }) {
    return <span className={styles.group}>{children}</span>;
}
