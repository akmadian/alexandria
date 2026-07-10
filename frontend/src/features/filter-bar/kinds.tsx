// The per-kind value-editor registry (frontend/09 "editors are per-kind; the pill
// is one generic component"). Each kind contributes a value SEGMENT (the tappable
// value part of the pill + its overlay), a `format` for the pill/narration text,
// and a `defaultValue` for a freshly added field. The pill is kind-agnostic and
// dispatches here (C10). New kind = one row; the pill never grows a branch.

import type { FC, ReactNode } from "react";
import { Button as AriaButton, Dialog, DialogTrigger } from "react-aria-components";
import { useTranslation } from "react-i18next";
import { Menu, MenuItem, MenuTrigger } from "@/components/menu/menu";
import { NumberField, TextField } from "@/components/field/field";
import { Popover } from "@/components/popover/popover";
import type { ValueKind } from "@/_generated-types/vocabulary";
import { formatNumber } from "@/lib/format";
import type { FilterField } from "./fields";
import s from "./filter-pill.module.css";

const PLACEHOLDER = "…"; // punctuation, not display copy — no i18n key

type Translate = (key: string) => string;

interface SegmentProps {
    def: FilterField;
    value: unknown;
    onValueChange: (value: unknown) => void;
}

export interface KindEditor {
    ValueSegment: FC<SegmentProps>;
    format: (value: unknown, def: FilterField, t: Translate) => string;
    defaultValue: (def: FilterField) => unknown;
}

// A field-backed value segment (numeric / text): a trigger showing the formatted
// value that opens a Popover holding the input. Shared shell so each kind supplies
// only its control.
function FieldSegment({ label, formatted, children }: { label: string; formatted: string; children: ReactNode }) {
    const { t } = useTranslation();
    return (
        <DialogTrigger>
            <AriaButton className={s.valueSeg} aria-label={t("filter.editValue")}>
                <span className={s.value}>{formatted}</span>
            </AriaButton>
            <Popover>
                <Dialog className={s.editorPop} aria-label={label}>
                    {children}
                </Dialog>
            </Popover>
        </DialogTrigger>
    );
}

function EnumSegment({ def, value, onValueChange }: SegmentProps) {
    const { t } = useTranslation();
    const members = def.members ?? [];
    const selected = Array.isArray(value) ? (value as string[]) : [];
    const label = (member: string) => t(`${def.labelNs}.${member}`);
    return (
        <MenuTrigger>
            <AriaButton className={s.valueSeg} aria-label={t("filter.editValue")}>
                <span className={s.value}>{selected.map(label).join(", ") || PLACEHOLDER}</span>
            </AriaButton>
            <Menu
                selectionMode="multiple"
                selectedKeys={new Set(selected)}
                onSelectionChange={(sel) => onValueChange(sel === "all" ? [...members] : [...sel].map(String))}
            >
                {members.map((member) => (
                    <MenuItem key={member} id={member}>
                        {label(member)}
                    </MenuItem>
                ))}
            </Menu>
        </MenuTrigger>
    );
}

function NumericSegment({ def, value, onValueChange }: SegmentProps) {
    const { t } = useTranslation();
    const num = typeof value === "number" ? value : Number.NaN;
    const fieldLabel = t(`filter.field.${def.field}`);
    return (
        <FieldSegment label={fieldLabel} formatted={Number.isFinite(num) ? formatNumber(num) : PLACEHOLDER}>
            <NumberField value={num} onChange={onValueChange} ariaLabel={fieldLabel} minValue={0} />
        </FieldSegment>
    );
}

function TextSegment({ def, value, onValueChange }: SegmentProps) {
    const { t } = useTranslation();
    const text = typeof value === "string" ? value : "";
    const fieldLabel = t(`filter.field.${def.field}`);
    return (
        <FieldSegment label={fieldLabel} formatted={text || PLACEHOLDER}>
            {/* ponytail: text commits per keystroke → a query each keypress; fine on
                the 80ms mock. TRIGGER: debounce when the Wails adapter binds. */}
            <TextField value={text} onChange={onValueChange} ariaLabel={fieldLabel} />
        </FieldSegment>
    );
}

const KIND_EDITORS: Partial<Record<ValueKind, KindEditor>> = {
    enum: {
        ValueSegment: EnumSegment,
        format: (value, def, t) => (Array.isArray(value) ? (value as string[]).map((m) => t(`${def.labelNs}.${m}`)).join(", ") : ""),
        defaultValue: (def) => [def.members?.[0]],
    },
    numeric: {
        ValueSegment: NumericSegment,
        format: (value) => (typeof value === "number" && Number.isFinite(value) ? formatNumber(value) : PLACEHOLDER),
        defaultValue: () => 0,
    },
    text: {
        ValueSegment: TextSegment,
        format: (value) => (typeof value === "string" && value ? value : PLACEHOLDER),
        defaultValue: () => "",
    },
};

/** The editor for a value kind, or undefined when the bar can't yet edit it (date /
 *  tag / source land as their editors are built). */
export function kindEditorFor(kind: ValueKind): KindEditor | undefined {
    return KIND_EDITORS[kind];
}
