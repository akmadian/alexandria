import { CameraLens, File02, Flag01, Image01, SlashCircle01, Star01, VideoRecorder } from "@untitledui/icons";
import type { FC } from "react";
import { colorLabelClass, type ColorLabel, type FileStatus, type FileType, type Flag } from "@/lib/mock";
import { cx } from "@/utils/cx";

export const fileTypeIcon: Record<FileType, FC<{ className?: string }>> = {
    image: Image01,
    raw: CameraLens,
    video: VideoRecorder,
    vector: File02,
    document: File02,
    audio: File02,
};

/** Interactive when `onRate` is given, otherwise read-only display. */
export const RatingStars = ({
    value,
    size = "sm",
    onRate,
    className,
}: {
    value: number;
    size?: "sm" | "md";
    onRate?: (v: number) => void;
    className?: string;
}) => {
    const px = size === "md" ? "size-4" : "size-3";
    return (
        <div className={cx("flex items-center gap-0.5", className)}>
            {[1, 2, 3, 4, 5].map((i) => {
                const active = i <= value;
                const star = (
                    <Star01
                        className={cx(px, active ? "fill-current text-yellow-400" : "text-fg-quaternary")}
                        aria-hidden="true"
                    />
                );
                return onRate ? (
                    <button
                        key={i}
                        type="button"
                        aria-label={`${i} star${i > 1 ? "s" : ""}`}
                        onClick={() => onRate(value === i ? 0 : i)}
                        className="rounded-xs outline-focus-ring transition duration-100 ease-linear hover:scale-110 focus-visible:outline-2"
                    >
                        {star}
                    </button>
                ) : (
                    <span key={i}>{star}</span>
                );
            })}
        </div>
    );
};

export const ColorDot = ({ label, className }: { label: ColorLabel; className?: string }) => (
    <span className={cx("inline-block size-2.5 rounded-full ring-1 ring-inset ring-black/10", colorLabelClass[label], className)} />
);

export const FlagBadge = ({ flag }: { flag: Flag }) => {
    if (!flag) return null;
    return flag === "pick" ? (
        <Flag01 className="size-3.5 text-success-primary" aria-label="Pick" />
    ) : (
        <SlashCircle01 className="size-3.5 text-error-primary" aria-label="Reject" />
    );
};

const statusStyle: Record<FileStatus, { dot: string; label: string }> = {
    online: { dot: "bg-success-500", label: "Online" },
    offline: { dot: "bg-gray-400", label: "Offline" },
    missing: { dot: "bg-error-500", label: "Missing" },
};

export const StatusDot = ({ status, withLabel = false }: { status: FileStatus; withLabel?: boolean }) => {
    const s = statusStyle[status];
    return (
        <span className="inline-flex items-center gap-1.5">
            <span className={cx("size-2 rounded-full", s.dot)} />
            {withLabel && <span className="text-xs text-tertiary">{s.label}</span>}
        </span>
    );
};
