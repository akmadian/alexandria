// The filter pill — one rendered leaf (C1), kind-agnostic. It draws the field, a
// generic operator segment (the token's allowed operators), and delegates the value
// segment to the per-kind editor registry (kinds.tsx). It emits a new Leaf on any
// edit; the bar owns tree assembly + dispatch. The pill has no per-kind branches —
// that lives in the registry.

import { ChevronDown, Flag, Image, Palette, Ruler, Star, Tag, Type as TypeIcon, X } from "lucide-react";
import type { ReactNode } from "react";
import { Button as AriaButton, type Selection } from "react-aria-components";
import { useTranslation } from "react-i18next";
import { Menu, MenuItem, MenuTrigger } from "@/components/menu/menu";
import type { TokenField, TokenOperator } from "@/_generated-types/vocabulary";
import type { Leaf } from "@/query-model/ast";
import { leaf, tokens, validate, valuelessOperator } from "@/query-model/registry";
import type { FilterField } from "./fields";
import s from "./filter-pill.module.css";
import { kindEditorFor } from "./kinds";

const FIELD_GLYPH: Partial<Record<TokenField, ReactNode>> = {
    rating: <Star size={13} />,
    flag: <Flag size={13} />,
    colorLabel: <Palette size={13} />,
    fileType: <Image size={13} />,
    filename: <TypeIcon size={13} />,
    cameraMake: <Ruler size={13} />,
    cameraModel: <Ruler size={13} />,
    title: <Tag size={13} />,
};

function firstOperator(selection: Selection): TokenOperator | undefined {
    if (selection === "all") return undefined;
    const [key] = selection;
    return typeof key === "string" ? (key as TokenOperator) : undefined;
}

function OperatorSegment({
    operators,
    value,
    onChange,
}: {
    operators: readonly TokenOperator[];
    value: TokenOperator;
    onChange: (operator: TokenOperator) => void;
}) {
    const { t } = useTranslation();
    return (
        <MenuTrigger>
            <AriaButton className={s.op} aria-label={t("filter.editOperator")}>
                {t(`filter.opShort.${value}`)}
                <span className={s.caret}>
                    <ChevronDown size={11} />
                </span>
            </AriaButton>
            <Menu
                selectionMode="single"
                disallowEmptySelection
                selectedKeys={new Set([value])}
                onSelectionChange={(sel) => {
                    const next = firstOperator(sel);
                    if (next) onChange(next);
                }}
            >
                {operators.map((operator) => (
                    <MenuItem key={operator} id={operator}>
                        {t(`filter.op.${operator}`)}
                    </MenuItem>
                ))}
            </Menu>
        </MenuTrigger>
    );
}

interface FilterPillProps {
    def: FilterField;
    node: Leaf;
    onChange: (next: Leaf) => void;
    onRemove: () => void;
}

export function FilterPill({ def, node, onChange, onRemove }: FilterPillProps) {
    const { t } = useTranslation();
    const editor = kindEditorFor(def.kind);
    const operators = tokens[def.field].operators;
    const showsValue = !valuelessOperator(node.cmp) && editor !== undefined;

    const changeOperator = (operator: TokenOperator) => {
        if (valuelessOperator(operator)) return onChange(leaf(def.field, operator, null));
        // Coming from a valueless operator, there's no value to keep — seed the
        // kind's default so the new leaf is valid immediately.
        const value = valuelessOperator(node.cmp) ? (editor?.defaultValue(def) ?? null) : node.value;
        onChange(leaf(def.field, operator, value));
    };

    return (
        <div className={s.pill} data-invalid={!validate(node)}>
            <span className={s.field}>
                <span className={s.glyph}>{FIELD_GLYPH[def.field]}</span>
                {t(`filter.field.${def.field}`)}
            </span>
            <OperatorSegment operators={operators} value={node.cmp} onChange={changeOperator} />
            {showsValue && editor && (
                <editor.ValueSegment
                    def={def}
                    value={node.value}
                    onValueChange={(value) => onChange(leaf(def.field, node.cmp, value))}
                />
            )}
            <AriaButton className={s.remove} aria-label={t("filter.remove")} onPress={onRemove}>
                <X size={13} />
            </AriaButton>
        </div>
    );
}

/** Inert pill for a leaf the bar can't edit yet (a kind with no editor, or an
 *  unknown token). Honors the D20 trust rule — shown and removable, never dropped. */
export function UnknownPill({ label, valueText, onRemove }: { label: string; valueText: string; onRemove: () => void }) {
    const { t } = useTranslation();
    return (
        <div className={s.pill} data-invalid="true">
            <span className={s.field}>{label}</span>
            <span className={s.valueStatic}>
                <span className={s.value}>{valueText}</span>
            </span>
            <AriaButton className={s.remove} aria-label={t("filter.remove")} onPress={onRemove}>
                <X size={13} />
            </AriaButton>
        </div>
    );
}
