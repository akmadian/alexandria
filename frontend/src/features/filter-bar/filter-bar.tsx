// Top bar: view title, search (debounced 200ms), type / rating / sort controls,
// density + theme toggles, result count. Controlled entirely by LibraryProvider
// state — this component fires no queries.

import { Grid2x2, LayoutGrid, Palette, Search, Star } from "lucide-react";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { cycleTheme } from "@/lib/theme";
import { Button, Toggle } from "@/components/button/button";
import { Icon } from "@/components/icon/icon";
import { InputField } from "@/components/input-field/input-field";
import { Select } from "@/components/select/select";
import { useLibraryDispatch, useLibraryState, type SortKey } from "@/app/library-state";
import { useCollections, useSources, useTags } from "@/api/queries";
import type { FileType } from "@/api/contract";
import s from "./filter-bar.module.css";

const FILE_TYPES: (FileType | "all")[] = ["all", "image", "raw", "video", "vector", "document", "audio"];
const SORT_KEYS: SortKey[] = ["captured-desc", "captured-asc", "rating-desc", "name-asc", "size-desc"];

export const FilterBar = ({ total }: { total: number }) => {
    const { t } = useTranslation();
    const state = useLibraryState();
    const dispatch = useLibraryDispatch();
    const { filters, target } = state;

    // Debounce free-text into the shared state; discrete controls fire immediately.
    const [search, setSearch] = useState(filters.search);
    // When the shared value changes externally (target switch clears it), adopt it.
    // Derive-during-render, not an effect — the documented "adjust state on prop change" pattern.
    const [prevExternal, setPrevExternal] = useState(filters.search);
    if (filters.search !== prevExternal) {
        setPrevExternal(filters.search);
        setSearch(filters.search);
    }
    useEffect(() => {
        const id = setTimeout(() => {
            if (search !== filters.search) dispatch({ type: "setFilters", patch: { search } });
        }, 200);
        return () => clearTimeout(id);
    }, [search, filters.search, dispatch]);

    const sources = useSources().data ?? [];
    const collections = useCollections().data ?? [];
    const tags = useTags().data ?? [];
    const title =
        target.kind === "all"
            ? t("browser.allAssets")
            : target.kind === "recent"
              ? t("browser.recent")
              : target.kind === "source"
                ? (sources.find((x) => x.id === target.id)?.name ?? "")
                : target.kind === "collection"
                  ? (collections.find((x) => x.id === target.id)?.name ?? "")
                  : (tags.find((x) => x.id === target.id)?.name ?? "");

    return (
        <div className={s.bar}>
            <h1 className={s.title}>{title}</h1>
            <span className={`${s.count} u-data`}>{t("filterBar.count", { count: total })}</span>

            <div className={s.search}>
                <Icon icon={Search} size={14} className={s.searchIcon} />
                <InputField aria-label={t("filterBar.searchPlaceholder")} placeholder={t("filterBar.searchPlaceholder")} value={search} onChange={setSearch} />
            </div>

            <Select
                aria-label={t("filterBar.fileType.all")}
                options={FILE_TYPES.map((ft) => ({ id: ft, label: ft === "all" ? t("filterBar.fileType.all") : t(`fileType.${ft}`) }))}
                value={filters.fileType}
                onChange={(id) => dispatch({ type: "setFilters", patch: { fileType: id as FileType | "all" } })}
            />

            <div className={s.stars} role="group" aria-label={t("filterBar.minRating")}>
                {[1, 2, 3, 4, 5].map((n) => (
                    <Button
                        key={n}
                        size="sm"
                        className={s.star}
                        aria-label={`≥ ${n}`}
                        onPress={() => dispatch({ type: "setFilters", patch: { minRating: filters.minRating === n ? 0 : n } })}
                    >
                        <Star size={13} strokeWidth={1.5} fill={n <= filters.minRating ? "var(--accent)" : "none"} color={n <= filters.minRating ? "var(--accent)" : "currentColor"} />
                    </Button>
                ))}
            </div>

            <Select
                aria-label={t("filterBar.sort.captured-desc")}
                options={SORT_KEYS.map((k) => ({ id: k, label: t(`filterBar.sort.${k}`) }))}
                value={filters.sort}
                onChange={(id) => dispatch({ type: "setFilters", patch: { sort: id as SortKey } })}
            />

            <Toggle
                size="sm"
                aria-label="Density"
                isSelected={filters.density === "compact"}
                onChange={(compact) => dispatch({ type: "setFilters", patch: { density: compact ? "compact" : "comfortable" } })}
            >
                <Icon icon={filters.density === "compact" ? Grid2x2 : LayoutGrid} size={14} />
            </Toggle>

            <Button size="sm" aria-label={t("filterBar.theme")} onPress={() => cycleTheme()}>
                <Icon icon={Palette} size={14} />
            </Button>
        </div>
    );
};
