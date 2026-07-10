// Menu primitive — React Aria's Menu (typeahead, roving focus, single/multi
// selection, dismiss) skinned by the DS spec. MenuTrigger wraps trigger + Popover
// so features write <MenuTrigger><Button/><Menu>…</Menu></MenuTrigger>; the glass
// floater, positioning, and keyboard are RAC's, not a hand-rolled fixed div.

import { Check } from "lucide-react";
import { Children, type ReactElement, type ReactNode } from "react";
import {
    Menu as AriaMenu,
    MenuItem as AriaMenuItem,
    MenuSection as AriaMenuSection,
    MenuTrigger as AriaMenuTrigger,
    Header,
    type MenuItemProps as AriaMenuItemProps,
    type MenuProps as AriaMenuProps,
    type MenuSectionProps,
    type MenuTriggerProps,
    Separator as AriaSeparator,
} from "react-aria-components";
import { Popover } from "@/components/popover/popover";
import { cx } from "@/lib/cx";
import s from "./menu.module.css";

/** Trigger + its menu. First child is the pressable trigger; second is <Menu>. */
export function MenuTrigger(props: MenuTriggerProps) {
    const [trigger, menu] = Children.toArray(props.children) as [ReactElement, ReactElement];
    return (
        <AriaMenuTrigger {...props}>
            {trigger}
            <Popover>{menu}</Popover>
        </AriaMenuTrigger>
    );
}

export function Menu<T extends object>({ className, ...rest }: Omit<AriaMenuProps<T>, "className"> & { className?: string }) {
    return <AriaMenu {...rest} className={cx(s.menu, className)} />;
}

export interface MenuItemProps extends Omit<AriaMenuItemProps, "className" | "children"> {
    children: ReactNode;
    icon?: ReactNode;
    danger?: boolean;
    className?: string;
}

export function MenuItem({ children, icon, danger, textValue, className, ...rest }: MenuItemProps) {
    // RAC can't infer textValue for typeahead once we wrap children in a render
    // prop, so derive it from string children (features pass i18n labels).
    const resolvedTextValue = textValue ?? (typeof children === "string" ? children : undefined);
    return (
        <AriaMenuItem {...rest} textValue={resolvedTextValue} className={cx(s.item, danger && s.danger, className)}>
            {({ isSelected, selectionMode }) => (
                <>
                    {selectionMode !== "none" && <span className={s.check}>{isSelected ? <Check size={14} /> : null}</span>}
                    {icon !== undefined && <span className={s.ic}>{icon}</span>}
                    <span className={s.label}>{children}</span>
                </>
            )}
        </AriaMenuItem>
    );
}

export function MenuSection<T extends object>(props: MenuSectionProps<T>) {
    return <AriaMenuSection {...props} />;
}

export function MenuHeading({ children }: { children: ReactNode }) {
    return <Header className={s.heading}>{children}</Header>;
}

export function MenuSeparator() {
    return <AriaSeparator className={s.separator} />;
}
