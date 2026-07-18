// ToggleButton — the second leaf (frontend/CLAUDE.md §6): the pressed-in
// toolbar/filter toggle. Quiet ghost shape at rest; ON is the register fill
// (surface.selected — the ratified +2 findable step), never hue as the sole
// signal (§4/§14): chrome stays lawful with the accent unset. Button's rung
// ladder does not carry over — a toggle's prominence is its boolean state.

import {
    ToggleButton as AriaToggleButton,
    type ToggleButtonProps as AriaToggleButtonProps,
} from "react-aria-components";
import { cx } from "@/lib/cx";
import styles from "./toggle-button.module.css";

export type ToggleButtonSize = "control" | "control-lg";

// C10: exhaustive by construction, mirroring Button.
const SIZE_CLASSES = {
    control: styles.control,
    "control-lg": styles.controlLarge,
} as const satisfies Record<ToggleButtonSize, string>;

export interface ToggleButtonProps extends Omit<AriaToggleButtonProps, "className" | "style"> {
    /** control = 24px (the dense-tool default); control-lg = 28px. */
    size?: ToggleButtonSize;
    className?: string;
}

export function ToggleButton({ size = "control", className, ...ariaProps }: ToggleButtonProps) {
    return (
        <AriaToggleButton
            {...ariaProps}
            className={cx(styles.toggleButton, SIZE_CLASSES[size], className)}
        />
    );
}
