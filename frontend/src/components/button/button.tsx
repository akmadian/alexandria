import { Button as AriaButton, ToggleButton as AriaToggleButton, type ButtonProps as AriaButtonProps, type ToggleButtonProps } from "react-aria-components";
import { cx } from "@/lib/cx";
import s from "./button.module.css";

// RAC Button: press/hover/focus states arrive as data attributes the CSS keys on.
type Variant = "primary" | "ghost" | "danger";
type Size = "sm" | "md";

interface ButtonProps extends Omit<AriaButtonProps, "className"> {
    variant?: Variant;
    size?: Size;
    className?: string;
}

export const Button = ({ variant = "ghost", size = "md", className, ...rest }: ButtonProps) => (
    <AriaButton {...rest} className={cx(s.button, s[variant], s[size], className)} />
);

/** Two-state button (filter toggles, pane toggles). Same look, `data-selected` when on. */
interface ToggleProps extends Omit<ToggleButtonProps, "className"> {
    size?: Size;
    className?: string;
}

export const Toggle = ({ size = "md", className, ...rest }: ToggleProps) => (
    <AriaToggleButton {...rest} className={cx(s.button, s.ghost, s.toggle, s[size], className)} />
);
