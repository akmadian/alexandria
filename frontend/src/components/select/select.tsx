import { ChevronDown } from "lucide-react";
import {
    Button as AriaButton,
    ListBox as AriaListBox,
    ListBoxItem as AriaListBoxItem,
    Popover as AriaPopover,
    Select as AriaSelect,
    SelectValue as AriaSelectValue,
    type Key,
} from "react-aria-components";
import { cx } from "@/lib/cx";
import { Icon } from "@/components/icon/icon";
import s from "./select.module.css";

export interface SelectOption {
    id: string;
    label: string;
}

interface SelectProps {
    "aria-label": string;
    options: SelectOption[];
    value: string;
    onChange: (id: string) => void;
    className?: string;
}

export const Select = ({ options, value, onChange, className, ...aria }: SelectProps) => (
    <AriaSelect {...aria} selectedKey={value} onSelectionChange={(k: Key | null) => k != null && onChange(String(k))} className={cx(s.select, className)}>
        <AriaButton className={s.trigger}>
            <AriaSelectValue className={s.value} />
            <Icon icon={ChevronDown} size={14} />
        </AriaButton>
        <AriaPopover className={s.popover} offset={2}>
            <AriaListBox className={s.listbox} items={options}>
                {(o) => (
                    <AriaListBoxItem id={o.id} textValue={o.label} className={s.option}>
                        {o.label}
                    </AriaListBoxItem>
                )}
            </AriaListBox>
        </AriaPopover>
    </AriaSelect>
);
