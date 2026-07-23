// Popover — the §6 transient shell (frontend/CLAUDE.md §6: RAC owns overlay positioning,
// focus scope, and dismiss; this carries only the look). The one shell every transient
// tenant shares — Select, Menu + its submenus, and future bespoke filter popovers — so the
// menu-vs-bespoke line (drawn at interaction semantics, not appearance) never shows as a
// visual seam. Extracted from Select when Menu became the second tenant. Per-tenant extras
// (Select's trigger-width floor, Menu's min-width) ride an added className, never this base.

import { Popover as AriaPopover, type PopoverProps as AriaPopoverProps } from "react-aria-components";
import { cx } from "@/lib/cx";
import styles from "./popover.module.css";

export interface PopoverProps extends Omit<AriaPopoverProps, "className"> {
    className?: string;
}

export function Popover({ className, offset = 4, ...props }: PopoverProps) {
    return <AriaPopover {...props} offset={offset} className={cx(styles.popover, className)} />;
}
