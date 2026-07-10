// The filter bar — the flat pill row that IS the query's top-level predicate
// (frontend/03). It reads the store filter, renders one generic pill per top-level
// leaf, and owns tree assembly: every edit computes a new WhereNode through the pure
// query-model assembler and dispatches one filter-replaced. The grid re-queries live.

import { Plus } from "lucide-react";
import type { Key } from "react-aria-components";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/button/button";
import { Menu, MenuHeading, MenuItem, MenuSection, MenuTrigger } from "@/components/menu/menu";
import type { TokenField, TokenOperator } from "@/_generated-types/vocabulary";
import { addLeaf, removeLeaf, replaceLeaf, topLevelLeaves } from "@/query-model/assemble";
import type { WhereNode } from "@/query-model/ast";
import { leaf, tokens, valuelessOperator } from "@/query-model/registry";
import { useCatalogDispatch, useFilter } from "@/stores/catalog-store";
import { CATEGORY_ORDER, type FilterField, FILTER_FIELDS, filterFieldFor } from "./fields";
import s from "./filter-bar.module.css";
import { FilterPill, UnknownPill } from "./filter-pill";
import { kindEditorFor } from "./kinds";

// The first operator that takes a value (fall back to the first if a field is
// absence-only) — the sensible default for a freshly added field.
const defaultOperator = (operators: readonly TokenOperator[]): TokenOperator =>
    operators.find((operator) => !valuelessOperator(operator)) ?? operators[0];

export function FilterBar() {
    const { t } = useTranslation();
    const filter = useFilter();
    const dispatch = useCatalogDispatch();
    const leaves = topLevelLeaves(filter);
    const replace = (next: WhereNode | null) => dispatch({ type: "filter-replaced", filter: next });

    const addField = (def: FilterField) => {
        const operator = defaultOperator(tokens[def.field].operators);
        const value = valuelessOperator(operator) ? null : (kindEditorFor(def.kind)?.defaultValue(def) ?? null);
        replace(addLeaf(filter, leaf(def.field, operator, value)));
    };

    return (
        <div className={s.bar}>
            {leaves.map((node, index) => {
                const def = filterFieldFor(node.field);
                const key = `${node.field}-${index}`;
                if (!def || !kindEditorFor(def.kind)) {
                    return (
                        <UnknownPill
                            key={key}
                            label={def ? t(`filter.field.${node.field}`) : node.field}
                            valueText={String(node.value)}
                            onRemove={() => replace(removeLeaf(filter, index))}
                        />
                    );
                }
                return (
                    <FilterPill
                        key={key}
                        def={def}
                        node={node}
                        onChange={(next) => replace(replaceLeaf(filter, index, next))}
                        onRemove={() => replace(removeLeaf(filter, index))}
                    />
                );
            })}

            <MenuTrigger>
                <Button variant="ghost" size="sm" aria-label={t("filter.add")}>
                    <Plus size={13} />
                    {t("filter.add")}
                </Button>
                <Menu
                    onAction={(key: Key) => {
                        const def = filterFieldFor(String(key) as TokenField);
                        if (def) addField(def);
                    }}
                >
                    {CATEGORY_ORDER.map((category) => {
                        const inCategory = FILTER_FIELDS.filter((field) => field.category === category);
                        if (inCategory.length === 0) return null;
                        return (
                            <MenuSection key={category}>
                                <MenuHeading>{t(`filter.category.${category}`)}</MenuHeading>
                                {inCategory.map((field) => (
                                    <MenuItem key={field.field} id={field.field}>
                                        {t(`filter.field.${field.field}`)}
                                    </MenuItem>
                                ))}
                            </MenuSection>
                        );
                    })}
                </Menu>
            </MenuTrigger>

            {filter !== null && (
                <>
                    <span className={s.spacer} />
                    <Button variant="ghost" size="sm" onPress={() => replace(null)}>
                        {t("filter.clear")}
                    </Button>
                </>
            )}
        </div>
    );
}
