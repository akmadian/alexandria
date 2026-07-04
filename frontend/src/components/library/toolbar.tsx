import { Grid01, LayoutGrid01, SearchLg, SwitchVertical01 } from "@untitledui/icons";
import { Input } from "@/components/base/input/input";
import { Select } from "@/components/base/select/select";
import { RatingStars } from "./bits";
import { cx } from "@/utils/cx";

export type Density = "comfortable" | "compact";
export type SortKey = "captured-desc" | "captured-asc" | "rating-desc" | "name-asc" | "size-desc";

export interface Filters {
    search: string;
    fileType: string; // "all" | FileType
    minRating: number;
    sort: SortKey;
    density: Density;
}

const fileTypeItems = [
    { id: "all", label: "All types" },
    { id: "image", label: "Images" },
    { id: "raw", label: "RAW" },
    { id: "video", label: "Video" },
    { id: "vector", label: "Vector" },
    { id: "document", label: "Documents" },
];

const sortItems = [
    { id: "captured-desc", label: "Newest first" },
    { id: "captured-asc", label: "Oldest first" },
    { id: "rating-desc", label: "Highest rated" },
    { id: "name-asc", label: "Name (A–Z)" },
    { id: "size-desc", label: "Largest file" },
];

export const Toolbar = ({
    title,
    count,
    filters,
    onChange,
}: {
    title: string;
    count: number;
    filters: Filters;
    onChange: (f: Partial<Filters>) => void;
}) => {
    return (
        <div className="sticky top-0 z-10 border-b border-secondary bg-primary/95 backdrop-blur-sm">
            <div className="flex items-center justify-between px-5 pt-4 pb-3">
                <div>
                    <h1 className="text-lg font-semibold text-primary">{title}</h1>
                    <p className="text-sm text-tertiary">{count.toLocaleString()} assets</p>
                </div>
                <div className="w-72">
                    <Input
                        size="sm"
                        aria-label="Search assets"
                        placeholder="Search filename, camera, location…"
                        icon={SearchLg}
                        value={filters.search}
                        onChange={(v) => onChange({ search: v })}
                    />
                </div>
            </div>

            <div className="flex items-center gap-2 px-5 pb-3">
                <div className="w-36">
                    <Select
                        size="sm"
                        aria-label="File type"
                        selectedKey={filters.fileType}
                        onSelectionChange={(k) => onChange({ fileType: String(k) })}
                        items={fileTypeItems}
                    >
                        {(item) => <Select.Item id={item.id}>{item.label}</Select.Item>}
                    </Select>
                </div>

                <div className="w-44">
                    <Select
                        size="sm"
                        aria-label="Sort by"
                        icon={SwitchVertical01}
                        selectedKey={filters.sort}
                        onSelectionChange={(k) => onChange({ sort: k as SortKey })}
                        items={sortItems}
                    >
                        {(item) => <Select.Item id={item.id}>{item.label}</Select.Item>}
                    </Select>
                </div>

                <div className="flex items-center gap-2 rounded-lg bg-secondary px-2.5 py-1.5">
                    <span className="text-xs font-medium text-tertiary">Min</span>
                    <RatingStars value={filters.minRating} onRate={(v) => onChange({ minRating: v })} />
                </div>

                <div className="ml-auto flex items-center gap-1 rounded-lg bg-secondary p-0.5">
                    {(
                        [
                            ["comfortable", LayoutGrid01],
                            ["compact", Grid01],
                        ] as const
                    ).map(([key, Icon]) => (
                        <button
                            key={key}
                            type="button"
                            aria-label={key}
                            aria-pressed={filters.density === key}
                            onClick={() => onChange({ density: key })}
                            className={cx(
                                "flex size-7 items-center justify-center rounded-md transition duration-100 ease-linear",
                                filters.density === key ? "bg-primary text-fg-secondary shadow-xs" : "text-fg-quaternary hover:text-fg-secondary",
                            )}
                        >
                            <Icon className="size-4" />
                        </button>
                    ))}
                </div>
            </div>
        </div>
    );
};
