// Button primitive — React Aria's Button (press/hover/focus across mouse, touch,
// keyboard, screen readers) skinned by the DS spec. Domain-blind chrome; features
// compose it. The DS variant/size classes are exhaustive registries (C10) so an
// unknown variant is a type error, not a silent default.

import { Button as AriaButton, type ButtonProps as AriaButtonProps } from "react-aria-components";
import { cx } from "@/lib/cx";
import s from "./button.module.css";

type Variant = "default" | "primary" | "ghost" | "danger";
type Size = "default" | "sm" | "lg";

const VARIANT_CLASS = {
    default: null,
    primary: s.primary,
    ghost: s.ghost,
    danger: s.danger,
} satisfies Record<Variant, string | null>;

const SIZE_CLASS = {
    default: null,
    sm: s.sm,
    lg: s.lg,
} satisfies Record<Size, string | null>;

export interface ButtonProps extends Omit<AriaButtonProps, "className"> {
    variant?: Variant;
    size?: Size;
    /** Square icon-only button (width tracks the size's control height). */
    icon?: boolean;
    className?: string;
}

export function Button({ variant = "default", size = "default", icon = false, className, ...rest }: ButtonProps) {
    return (
        <AriaButton {...rest} className={cx(s.button, VARIANT_CLASS[variant], SIZE_CLASS[size], icon && s.icon, className)} />
    );
}
