// Menu — the §6 transient's second tenant (after Select): a role=menu roving list of
// commands/choices on the shared Popover shell (components/popover). The line we hold, the
// one every serious system draws (ARIA APG / Radix / RAC / AppKit): a Menu is a SINGLE
// roving focus over commands — anything needing multiple tab stops, embedded form controls,
// or a filtering field is a bespoke Popover (role=dialog), NOT a MenuItem. Header/footer
// chrome (search, tabs, footer buttons, button clusters) composes as siblings on the Popover,
// outside this roving list, so it never breaks the menu model.
//
// MenuItem's accessory model is three tiers, never a prop per combination (the §22
// combination-name defect in component form): AUTOMATIC markers (submenu chevron / selection
// check, from RAC render-props) · SUGAR for the common case (icon / shortcut) · COMPOSITION
// for the long tail. Item substrate = Select's item verbatim (one collection material). RAC
// SubmenuTrigger owns hover-open (safe-triangle, Right/Left, timing) — never hand-rolled.

import { Children, type ReactElement, type ReactNode } from "react";
import {
    Header as AriaHeader,
    Menu as AriaMenu,
    MenuItem as AriaMenuItem,
    type MenuItemProps as AriaMenuItemProps,
    type MenuProps as AriaMenuProps,
    MenuSection as AriaMenuSection,
    type MenuSectionProps as AriaMenuSectionProps,
    MenuTrigger as AriaMenuTrigger,
    type MenuTriggerProps as AriaMenuTriggerProps,
    Separator as AriaSeparator,
    SubmenuTrigger as AriaSubmenuTrigger,
    type SubmenuTriggerProps as AriaSubmenuTriggerProps,
    Text as AriaText,
} from "react-aria-components";
import { Icon, type IconConcept } from "@/components/icon/icon";
import { Kbd } from "@/components/kbd/kbd";
import { Popover } from "@/components/popover/popover";
import { cx } from "@/lib/cx";
import styles from "./menu.module.css";

// MenuTrigger wraps [trigger, menu] and injects the shared Popover shell (the RAC starter
// pattern) so consumers write `<MenuTrigger><Button/><Menu/></MenuTrigger>`.
export function MenuTrigger({ children, ...props }: AriaMenuTriggerProps) {
    const [trigger, menu] = Children.toArray(children) as [ReactElement, ReactElement];
    return (
        <AriaMenuTrigger {...props}>
            {trigger}
            <Popover>{menu}</Popover>
        </AriaMenuTrigger>
    );
}

export interface MenuProps<T extends object> extends Omit<AriaMenuProps<T>, "className"> {
    className?: string;
}

export function Menu<T extends object>({ className, ...props }: MenuProps<T>) {
    return <AriaMenu {...props} className={cx(styles.list, className)} />;
}

export interface MenuItemProps extends Omit<AriaMenuItemProps, "children" | "className"> {
    /** The item label (the roving-focus command). */
    children: ReactNode;
    /** Leading icon (sugar). Presence of ANY icon in a menu reserves the gutter for all rows. */
    icon?: IconConcept;
    /** Trailing keyboard shortcut (sugar). A bare string renders as a single flat `Kbd` keycap;
     * for a multi-key combo pass a composed `<KbdGroup>` of `<Kbd>` caps (⌘ ⇧ P). */
    shortcut?: ReactNode;
    /** A muted second line (native two-line item; the row grows — a sanctioned §8 exception). */
    description?: ReactNode;
    /** Structural danger seam — renders hue-free ink TODAY; the danger tone plugs in during the
     * signals-color hue round (D36). The `data-destructive` hook is the wiring point. */
    isDestructive?: boolean;
    className?: string;
}

export function MenuItem({
    children,
    icon,
    shortcut,
    description,
    isDestructive,
    className,
    ...props
}: MenuItemProps) {
    const textValue = props.textValue ?? (typeof children === "string" ? children : undefined);
    return (
        <AriaMenuItem
            {...props}
            textValue={textValue}
            data-destructive={isDestructive || undefined}
            className={cx(styles.item, className)}
        >
            {({ hasSubmenu, isSelected }) => (
                <>
                    <span className={styles.leading}>{icon && <Icon concept={icon} />}</span>
                    <span className={styles.body}>
                        <AriaText slot="label" className={styles.label}>
                            {children}
                        </AriaText>
                        {description !== undefined && (
                            <AriaText slot="description" className={styles.description}>
                                {description}
                            </AriaText>
                        )}
                    </span>
                    {(shortcut !== undefined || isSelected || hasSubmenu) && (
                        <span className={styles.trailing}>
                            {shortcut !== undefined &&
                                (typeof shortcut === "string" ? <Kbd>{shortcut}</Kbd> : shortcut)}
                            {isSelected && <Icon concept="check" className={styles.check} />}
                            {hasSubmenu && <Icon concept="disclose" className={styles.chevron} />}
                        </span>
                    )}
                </>
            )}
        </AriaMenuItem>
    );
}

export interface MenuSectionProps<T extends object> extends Omit<AriaMenuSectionProps<T>, "className"> {
    className?: string;
}

export function MenuSection<T extends object>({ className, ...props }: MenuSectionProps<T>) {
    return <AriaMenuSection {...props} className={cx(styles.section, className)} />;
}

// The group label — muted, sentence case (§9; NOT the references' uppercase micro-labels).
export function MenuSectionHeader({ children, className }: { children: ReactNode; className?: string }) {
    return <AriaHeader className={cx(styles.sectionHeader, className)}>{children}</AriaHeader>;
}

export function MenuSeparator({ className }: { className?: string }) {
    return <AriaSeparator className={cx(styles.separator, className)} />;
}

// SubmenuTrigger wraps [item, submenu] and injects a nested Popover on the same shell. RAC
// owns the hover-open behavior; the chevron appears automatically via the item's `hasSubmenu`.
export function SubmenuTrigger({ children, ...props }: AriaSubmenuTriggerProps) {
    const [item, submenu] = Children.toArray(children) as [ReactElement, ReactElement];
    // The submenu sits just OUTSIDE the parent's right edge (offset clears the parent popover's
    // padding + border so the two don't overlap); crossOffset lifts it so its first item aligns
    // with the parent trigger row. (Positioning px — RAC inputs, eye-tuned; not design tokens.)
    return (
        <AriaSubmenuTrigger {...props}>
            {item}
            <Popover offset={6} crossOffset={-5}>
                {submenu}
            </Popover>
        </AriaSubmenuTrigger>
    );
}
