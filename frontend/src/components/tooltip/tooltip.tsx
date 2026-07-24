// Tooltip — the §6 fixed-dark label (§24 tooltip row, D42). The one polarity exception:
// a fixed-dark chip on EVERY theme ("labels, not surfaces"), so a tooltip reads the same
// wherever it appears. RAC ships Tooltip as its OWN primitive (not on the Popover shell) —
// hover/focus with a warmup delay, non-interactive, never shown on touch — so this wraps RAC
// directly. RAC owns behavior (positioning, delay, dismiss); this carries only the look.
//
// Polarity is a closed strategy set (§22), never a per-instance knob: `dark` (default, the §6
// fixed-dark chip) and `inverse` (theme-contrasting — dark on light worlds, light on dark,
// built from each theme's own poles). Surface + ink are the seated tooltip tokens; the
// separating rim DERIVES from the variant ink at the seated alpha, so on dark chrome it reads
// as a light rim (the §6 separator there — shadow alone does not read on dark). A
// shortcut-carrying / keyboard-hint tooltip is a later variant (D42-deferred); the
// keycap-from-ink tokens are already seated for it.

import type { ReactNode } from "react";
import {
    Tooltip as AriaTooltip,
    type TooltipProps as AriaTooltipProps,
    TooltipTrigger as AriaTooltipTrigger,
    type TooltipTriggerComponentProps as AriaTooltipTriggerProps,
} from "react-aria-components";
import { cx } from "@/lib/cx";
import styles from "./tooltip.module.css";

export type TooltipVariant = "dark" | "inverse";

// C10 completeness: a new variant fails to compile until it maps to a class.
const VARIANT_CLASSES = {
    dark: styles.dark,
    inverse: styles.inverse,
} as const satisfies Record<TooltipVariant, string>;

export interface TooltipProps extends Omit<AriaTooltipProps, "className" | "children"> {
    children: ReactNode;
    /** Polarity (§24/D42): `dark` (default, the §6 fixed-dark chip) · `inverse` (theme-contrasting). */
    variant?: TooltipVariant;
    className?: string;
}

// offset defaults to the 5px trigger gap eye-gated in the tooltip round (RAC's default is 0).
// Every other RAC positioning prop (placement, crossOffset, shouldFlip, containerPadding, …)
// passes straight through.
export function Tooltip({ children, variant = "dark", offset = 5, className, ...props }: TooltipProps) {
    return (
        <AriaTooltip {...props} offset={offset} className={cx(styles.tooltip, VARIANT_CLASSES[variant], className)}>
            {children}
        </AriaTooltip>
    );
}

// TooltipTrigger — RAC owns warmup/hover/focus/dismiss; we set only a house warmup default
// (RAC's 1500ms reads sluggish for a dense tool; ~700ms is the common convention, e.g. Radix's
// default). delay/closeDelay/trigger/isOpen still pass through and override.
export function TooltipTrigger({ delay = 700, ...props }: AriaTooltipTriggerProps) {
    return <AriaTooltipTrigger delay={delay} {...props} />;
}
