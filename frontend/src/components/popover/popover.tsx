// Popover primitive — React Aria's Popover (positioning, focus containment,
// dismiss, portal) skinned as the DS glass surface. Menus, selects, and value
// editors mount their content inside it, so the surface chrome lives here once.

import { Popover as AriaPopover, type PopoverProps } from "react-aria-components";
import { cx } from "@/lib/cx";
import s from "./popover.module.css";

export function Popover({ className, ...rest }: Omit<PopoverProps, "className"> & { className?: string }) {
    return <AriaPopover {...rest} className={cx(s.popover, className)} />;
}
