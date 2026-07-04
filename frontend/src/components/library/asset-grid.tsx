import { WifiOff } from "@untitledui/icons";
import type { AssetRow } from "@/api/contract";
import { formatBytes } from "@/api/mock";
import { cx } from "@/utils/cx";
import { ColorDot, FlagBadge, fileTypeIcon, RatingStars } from "./bits";
import type { Density } from "./toolbar";

const AssetCard = ({
    asset,
    selected,
    onSelect,
    compact,
}: {
    asset: AssetRow;
    selected: boolean;
    onSelect: () => void;
    compact: boolean;
}) => {
    const TypeIcon = fileTypeIcon[asset.fileType];
    const offline = asset.fileStatus !== "online";

    return (
        <button
            type="button"
            onClick={onSelect}
            aria-pressed={selected}
            className={cx(
                "group flex flex-col overflow-hidden rounded-lg text-left ring-1 transition duration-100 ease-linear",
                selected ? "ring-2 ring-brand" : "ring-secondary hover:ring-primary",
            )}
        >
            <div className="relative aspect-[4/3] overflow-hidden bg-secondary">
                <img
                    src={asset.thumbURL}
                    alt={asset.filename}
                    loading="lazy"
                    className={cx(
                        "size-full object-cover transition duration-200 group-hover:scale-[1.03]",
                        offline && "opacity-40 grayscale",
                    )}
                />

                {/* Top overlays */}
                <div className="pointer-events-none absolute inset-x-0 top-0 flex items-start justify-between p-1.5">
                    <span className="flex items-center gap-1">
                        {asset.flag && (
                            <span className="flex size-5 items-center justify-center rounded-md bg-primary/90 shadow-xs backdrop-blur-sm">
                                <FlagBadge flag={asset.flag} />
                            </span>
                        )}
                        {(asset.fileType === "raw" || asset.fileType === "video") && (
                            <span className="rounded-md bg-black/55 px-1.5 py-0.5 text-[10px] font-semibold tracking-wide text-white uppercase backdrop-blur-sm">
                                {asset.extension}
                            </span>
                        )}
                    </span>
                    {asset.colorLabel && <ColorDot label={asset.colorLabel} className="size-3 ring-white/60" />}
                </div>

                {offline && (
                    <span className="absolute right-1.5 bottom-1.5 flex items-center gap-1 rounded-md bg-black/55 px-1.5 py-0.5 text-[10px] font-medium text-white backdrop-blur-sm">
                        <WifiOff className="size-3" /> Offline
                    </span>
                )}

                {asset.rating > 0 && (
                    <div className="absolute inset-x-0 bottom-0 flex items-end bg-gradient-to-t from-black/55 to-transparent p-1.5 pt-5">
                        <RatingStars value={asset.rating} />
                    </div>
                )}
            </div>

            <div className="flex items-center gap-1.5 bg-primary px-2 py-1.5">
                <TypeIcon className="size-3.5 shrink-0 text-fg-quaternary" />
                <p className="truncate text-xs font-medium text-secondary">{asset.filename}</p>
                {!compact && <span className="ml-auto shrink-0 text-[11px] text-quaternary tabular-nums">{formatBytes(asset.sizeBytes)}</span>}
            </div>
        </button>
    );
};

export const AssetGrid = ({
    assets,
    selectedId,
    onSelect,
    density,
}: {
    assets: AssetRow[];
    selectedId: string | null;
    onSelect: (id: string) => void;
    density: Density;
}) => {
    if (assets.length === 0) {
        return (
            <div className="flex flex-1 flex-col items-center justify-center gap-1 py-24 text-center">
                <p className="text-sm font-medium text-secondary">No assets match your filters</p>
                <p className="text-sm text-tertiary">Try clearing the search or lowering the minimum rating.</p>
            </div>
        );
    }

    const min = density === "compact" ? "140px" : "220px";
    return (
        <div
            className="grid gap-3 p-5"
            style={{ gridTemplateColumns: `repeat(auto-fill, minmax(${min}, 1fr))` }}
        >
            {assets.map((a) => (
                <AssetCard
                    key={a.id}
                    asset={a}
                    selected={a.id === selectedId}
                    onSelect={() => onSelect(a.id)}
                    compact={density === "compact"}
                />
            ))}
        </div>
    );
};
