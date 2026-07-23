// Select — the dropdown/select trigger (frontend/CLAUDE.md §6: RAC owns overlay behavior).
// The FIRST overlay primitive: the trigger rides the D35 control-container material (so a
// dropdown reads as the same recessed chip as a field), and the popover establishes the §6
// TRANSIENT grammar — surface.transient + the occlusion shadow + the transient radius rung,
// the only place docked-chrome's no-shadow rule lifts. Single-select; the trigger shows no
// visible label (a field / ControlRow names it), matching the reference inspector dropdowns.

import type { ReactNode } from "react";
import {
    Button as AriaButton,
    ListBox as AriaListBox,
    ListBoxItem as AriaListBoxItem,
    type ListBoxItemProps as AriaListBoxItemProps,
    Popover as AriaPopover,
    Select as AriaSelect,
    type SelectProps as AriaSelectProps,
    SelectValue as AriaSelectValue,
} from "react-aria-components";
import { Icon } from "@/components/icon/icon";
import { cx } from "@/lib/cx";
import styles from "./select.module.css";

export type SelectSize = "xs" | "sm" | "md" | "lg";

// C10: mirrors TextField — the trigger sizes on the control ladder.
const SIZE_CLASSES = {
    xs: styles.controlXsmall,
    sm: styles.controlSmall,
    md: styles.controlMedium,
    lg: styles.controlLarge,
} as const satisfies Record<SelectSize, string>;

export interface SelectProps<T extends object>
    extends Omit<AriaSelectProps<T>, "children" | "className" | "style"> {
    /** Accessible name — the trigger shows no visible label (a field / ControlRow names it). */
    label: string;
    /** §8 control ladder: xs 16 / sm 20 / md 24 / lg 28. */
    size?: SelectSize;
    /** The <SelectItem>s, or a render function paired with `items`. */
    children: ReactNode | ((item: T) => ReactNode);
    /** Dynamic collection (optional); pair with a children render function. */
    items?: Iterable<T>;
    className?: string;
}

export function Select<T extends object>({
    label,
    size = "md",
    children,
    items,
    className,
    ...ariaProps
}: SelectProps<T>) {
    return (
        <AriaSelect {...ariaProps} aria-label={label} className={cx(styles.select, className)}>
            <AriaButton className={cx(styles.trigger, SIZE_CLASSES[size])}>
                <AriaSelectValue className={styles.value} />
                <Icon concept="disclose" className={styles.chevron} />
            </AriaButton>
            <AriaPopover className={styles.popover} offset={4}>
                <AriaListBox className={styles.listbox} items={items}>
                    {children}
                </AriaListBox>
            </AriaPopover>
        </AriaSelect>
    );
}

export function SelectItem({ children, ...props }: Omit<AriaListBoxItemProps, "children"> & { children: ReactNode }) {
    return (
        <AriaListBoxItem
            {...props}
            textValue={typeof children === "string" ? children : undefined}
            className={styles.item}
        >
            {({ isSelected }) => (
                <>
                    <span className={styles.itemLabel}>{children}</span>
                    {isSelected && <Icon concept="check" className={styles.check} />}
                </>
            )}
        </AriaListBoxItem>
    );
}
