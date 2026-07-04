import { Camera01, Flag01, MarkerPin01, SlashCircle01, XClose } from "@untitledui/icons";
import { Badge } from "@/components/base/badges/badges";
import type { Asset, ColorLabel, Flag } from "@/api/contract";
import { colorLabelClass, formatBytes, formatDate, formatDateTime, tags as allTags, thumbUrl } from "@/api/mock";
import { cx } from "@/utils/cx";
import { RatingStars, StatusDot, fileTypeIcon } from "./bits";

const COLORS: ColorLabel[] = ["red", "orange", "yellow", "green", "blue", "purple"];

const Row = ({ label, value }: { label: string; value?: React.ReactNode }) => {
    if (value === undefined || value === null || value === "") return null;
    return (
        <div className="flex items-baseline justify-between gap-3 py-1">
            <dt className="shrink-0 text-xs text-quaternary">{label}</dt>
            <dd className="truncate text-right text-xs font-medium text-secondary tabular-nums">{value}</dd>
        </div>
    );
};

const Section = ({ title, icon: Icon, children }: { title: string; icon?: React.FC<{ className?: string }>; children: React.ReactNode }) => (
    <div className="border-t border-secondary px-4 py-3">
        <div className="mb-1 flex items-center gap-1.5">
            {Icon && <Icon className="size-3.5 text-fg-quaternary" />}
            <p className="text-xs font-semibold tracking-wide text-quaternary uppercase">{title}</p>
        </div>
        <dl>{children}</dl>
    </div>
);

export const Inspector = ({
    asset,
    onUpdate,
    onClose,
}: {
    asset: Asset | null;
    onUpdate: (patch: Partial<Asset>) => void;
    onClose: () => void;
}) => {
    if (!asset) {
        return (
            <aside className="hidden w-80 shrink-0 flex-col items-center justify-center border-l border-secondary bg-primary p-8 text-center xl:flex">
                <div className="mb-3 flex size-11 items-center justify-center rounded-full bg-secondary">
                    <Camera01 className="size-5 text-fg-quaternary" />
                </div>
                <p className="text-sm font-medium text-secondary">No asset selected</p>
                <p className="mt-1 text-sm text-tertiary">Select an asset to view its metadata and EXIF details.</p>
            </aside>
        );
    }

    const TypeIcon = fileTypeIcon[asset.fileType];
    const assetTags = allTags.filter((t) => asset.tagIds.includes(t.id));
    const exif = asset.aperture
        ? `ƒ/${asset.aperture} · ${asset.shutterSpeed}s · ISO ${asset.iso}`
        : undefined;

    const setFlag = (f: Flag) => onUpdate({ flag: asset.flag === f ? null : f });
    const setColor = (c: ColorLabel | null) => onUpdate({ colorLabel: asset.colorLabel === c ? null : c });

    return (
        <aside className="flex w-80 shrink-0 flex-col overflow-y-auto border-l border-secondary bg-primary">
            <div className="relative aspect-[4/3] shrink-0 bg-secondary">
                <img src={thumbUrl(asset, 640)} alt={asset.filename} className="size-full object-cover" />
                <button
                    type="button"
                    onClick={onClose}
                    aria-label="Close inspector"
                    className="absolute top-2 right-2 flex size-7 items-center justify-center rounded-md bg-black/50 text-white backdrop-blur-sm transition duration-100 hover:bg-black/70"
                >
                    <XClose className="size-4" />
                </button>
            </div>

            <div className="px-4 py-3">
                <div className="flex items-start gap-1.5">
                    <TypeIcon className="mt-0.5 size-4 shrink-0 text-fg-quaternary" />
                    <p className="text-sm font-semibold break-all text-primary">{asset.filename}</p>
                </div>
                <div className="mt-1 flex items-center gap-2 pl-5.5">
                    <StatusDot status={asset.fileStatus} withLabel />
                    <span className="text-xs text-quaternary">·</span>
                    <span className="text-xs text-tertiary">{asset.width.toLocaleString()} × {asset.height.toLocaleString()}</span>
                </div>

                {/* Rating */}
                <div className="mt-3">
                    <RatingStars value={asset.rating} size="md" onRate={(v) => onUpdate({ rating: v })} />
                </div>

                {/* Color labels + flags */}
                <div className="mt-3 flex items-center gap-3">
                    <div className="flex items-center gap-1">
                        {COLORS.map((c) => (
                            <button
                                key={c}
                                type="button"
                                aria-label={`${c} label`}
                                aria-pressed={asset.colorLabel === c}
                                onClick={() => setColor(c)}
                                className={cx(
                                    "size-4 rounded-full ring-1 ring-inset ring-black/10 transition duration-100 ease-linear hover:scale-110",
                                    colorLabelClass[c],
                                    asset.colorLabel === c && "ring-2 ring-offset-2 ring-offset-primary ring-fg-brand-primary",
                                )}
                            />
                        ))}
                    </div>
                    <div className="ml-auto flex items-center gap-1">
                        <button
                            type="button"
                            aria-label="Pick"
                            aria-pressed={asset.flag === "pick"}
                            onClick={() => setFlag("pick")}
                            className={cx(
                                "flex size-7 items-center justify-center rounded-md transition duration-100 ease-linear",
                                asset.flag === "pick" ? "bg-success-secondary text-success-primary" : "text-fg-quaternary hover:bg-secondary",
                            )}
                        >
                            <Flag01 className="size-4" />
                        </button>
                        <button
                            type="button"
                            aria-label="Reject"
                            aria-pressed={asset.flag === "reject"}
                            onClick={() => setFlag("reject")}
                            className={cx(
                                "flex size-7 items-center justify-center rounded-md transition duration-100 ease-linear",
                                asset.flag === "reject" ? "bg-error-secondary text-error-primary" : "text-fg-quaternary hover:bg-secondary",
                            )}
                        >
                            <SlashCircle01 className="size-4" />
                        </button>
                    </div>
                </div>
            </div>

            <Section title="File">
                <Row label="Type" value={asset.fileType.toUpperCase()} />
                <Row label="Format" value={asset.extension} />
                <Row label="Size" value={formatBytes(asset.sizeBytes)} />
                <Row label="Dimensions" value={`${asset.width} × ${asset.height}`} />
            </Section>

            {asset.cameraModel && (
                <Section title="Camera" icon={Camera01}>
                    <Row label="Body" value={`${asset.cameraMake} ${asset.cameraModel}`} />
                    <Row label="Lens" value={asset.lensModel} />
                    <Row label="Focal length" value={asset.focalLengthMM ? `${asset.focalLengthMM}mm` : undefined} />
                    <Row label="Exposure" value={exif} />
                </Section>
            )}

            <Section title="Capture" icon={MarkerPin01}>
                <Row label="Captured" value={formatDateTime(asset.capturedAt)} />
                <Row label="Location" value={asset.location} />
                <Row label="Creator" value={asset.creator} />
                <Row label="Added" value={formatDate(asset.capturedAt)} />
            </Section>

            <Section title="Tags">
                {assetTags.length === 0 ? (
                    <p className="py-1 text-xs text-tertiary">No tags</p>
                ) : (
                    <div className="flex flex-wrap gap-1.5 pt-1">
                        {assetTags.map((t) => (
                            <Badge key={t.id} size="sm" color="gray" type="modern">
                                {t.name}
                            </Badge>
                        ))}
                    </div>
                )}
            </Section>
        </aside>
    );
};
