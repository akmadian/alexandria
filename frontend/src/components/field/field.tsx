// Field primitives — React Aria's TextField / NumberField (labelling, validation,
// locale-aware number parsing) skinned by the DS Input spec. Domain-blind; the
// filter bar's value editors compose them. NumberField renders as a plain field
// (no steppers) — a filter value doesn't need increment buttons.

import { Input, NumberField as AriaNumberField, TextField as AriaTextField } from "react-aria-components";
import { cx } from "@/lib/cx";
import s from "./field.module.css";

// A value editor mounts inside a RAC Dialog, which moves focus to the first
// focusable (this input) on open — so no autoFocus needed (and it'd trip a11y lint).
interface TextFieldProps {
    value: string;
    onChange: (value: string) => void;
    ariaLabel: string;
    placeholder?: string;
    className?: string;
}

export function TextField({ value, onChange, ariaLabel, placeholder, className }: TextFieldProps) {
    return (
        <AriaTextField value={value} onChange={onChange} aria-label={ariaLabel} className={cx(s.field, className)}>
            <Input className={s.input} placeholder={placeholder} />
        </AriaTextField>
    );
}

interface NumberFieldProps {
    value: number;
    onChange: (value: number) => void;
    ariaLabel: string;
    placeholder?: string;
    minValue?: number;
    maxValue?: number;
    className?: string;
}

export function NumberField({ value, onChange, ariaLabel, placeholder, minValue, maxValue, className }: NumberFieldProps) {
    return (
        <AriaNumberField
            // A non-finite value (a just-added / cleared field) shows as empty.
            value={Number.isFinite(value) ? value : Number.NaN}
            onChange={onChange}
            minValue={minValue}
            maxValue={maxValue}
            aria-label={ariaLabel}
            formatOptions={{ maximumFractionDigits: 0 }}
            className={cx(s.field, className)}
        >
            <Input className={s.input} placeholder={placeholder} />
        </AriaNumberField>
    );
}
