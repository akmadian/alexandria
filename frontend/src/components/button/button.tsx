// Button primitive — React Aria's Button (press/hover/focus across mouse, touch,
// keyboard, screen readers) skinned by the v3 system. Variants are the §4
// prominence rungs; exhaustive registries (C10) so an unknown rung is a type
// error, not a silent default.

import { Button as AriaButton, type ButtonProps as AriaButtonProps } from "react-aria-components";
import { cx } from "@/lib/cx";
import s from "./button.module.css";

export type ButtonRung = "ghost" | "outline" | "tint" | "fill" | "hero";
type Size = "default" | "sm";

const RUNG_CLASS = {
    ghost: s.ghost,
    outline: s.outline,
    tint: s.tint,
    fill: s.fill,
    hero: s.hero,
} satisfies Record<ButtonRung, string>;

const SIZE_CLASS = {
    default: null,
    sm: s.sm,
} satisfies Record<Size, string | null>;

export interface ButtonProps extends Omit<AriaButtonProps, "className"> {
    /** Prominence rung (§4). Outline is the default control face. */
    rung?: ButtonRung;
    size?: Size;
    /** Square icon-only button (width tracks the size's control height). */
    icon?: boolean;
    className?: string;
}

export function Button({ rung = "outline", size = "default", icon = false, className, ...rest }: ButtonProps) {
    return (
        <AriaButton {...rest} className={cx(s.button, RUNG_CLASS[rung], SIZE_CLASS[size], icon && s.icon, className)} />
    );
}
