// StatusBar — the §12 bottom zone: the always-on readout band. Docked chrome — flat, hue-free, a
// single hairline seam on top, no shadow; the data-sm mono voice; 24px tall (the row-list rung).
//
// ONE content slot by design. The readout has three lanes — counts · active filename · machinery/
// transient (§15: `1,204 · 3 selected · _DSF4926.RAF`, plus `★★★ → 3 assets` fan-out) — but with a
// single consumer the feature composes them (it passes multiple children + a spacer into the slot),
// keeping this a pure shell. Pure readout: no interaction is baked in (§15, "the model is always
// observable"); a slot child MAY be a button, but the bar mandates nothing. Transient fan-out
// confirmations and their aria-live announcements are feature-level content, not this shell's job.
//
// ponytail: intentionally thin — a styled docked band + slot. Its value is centralizing the
// bottom-band look + height so every future bottom bar (Import progress, etc.) matches, plus the
// per-platform note below in one place.
//
// Per-platform (frameless future): the window is native-framed today, so the OS owns the bottom
// edge and there is nothing to reserve. When the header round flips it to Frameless for the macOS
// custom titlebar, the bottom-RIGHT corner becomes a resize affordance — the feature filling the
// slot must then keep interactive controls out of that corner. Noted here so it isn't rediscovered.

import type { ReactNode } from "react";
import { cx } from "@/lib/cx";
import styles from "./status-bar.module.css";

export interface StatusBarProps {
    /** Optional accessible name for the band (the consumer's readout content is the substance). */
    "aria-label"?: string;
    className?: string;
    children: ReactNode;
}

export function StatusBar({ "aria-label": label, className, children }: StatusBarProps) {
    return (
        <footer aria-label={label} className={cx(styles.bar, className)}>
            {children}
        </footer>
    );
}
