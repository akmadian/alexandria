// Inspector — the right rail's metadata panel (§12), read-only this round.
// The SUBJECT is the active asset (§15): the cursor, reactively — selection
// size never changes what renders here until the judgment round adds editing
// (whose editors land inside these sections; mixed-value display arrives with
// them). Data is the AssetDetail wire projection fetched per id; rows render
// only when a value exists and a section with no rows renders nothing — the
// cheap half of per-type adaptivity (the registry-driven inspector is P2).
//
// Judgment displays follow the §19 cell-face grammar: the rating readout is
// ALWAYS present (unrated = five hollow positions — the ratified exception to
// "normal is silent"), label swatch and flag appear only when set.

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { AssetDetail } from "@/api/contract";
import { useAsset } from "@/api/queries";
import { Button } from "@/components/button/button";
import { Icon } from "@/components/icon/icon";
import { LabelSwatch } from "@/components/label-swatch/label-swatch";
import { PanelSection } from "@/components/panel-section/panel-section";
import { Rating } from "@/components/rating/rating";
import { Row } from "@/components/row/row";
import { cx } from "@/lib/cx";
import {
    formatBytes,
    formatDateTime,
    formatDuration,
    formatExposure,
    formatFocalLength,
    formatGps,
} from "@/lib/format";
import { useCursorId } from "@/stores/catalog-store";
import styles from "./inspector.module.css";

/**
 * §13: filenames middle-truncate — information lives at both ends (Finder
 * convention). CSS owns width adaptation: the head flexes and ellipsizes, the
 * rigid tail keeps the extension and trailing characters visible. Exported for
 * tests; promotes to components/ when a second surface needs it.
 */
export function MiddleTruncate({ text }: { text: string }) {
    const TAIL_CHARACTERS = 9;
    if (text.length <= TAIL_CHARACTERS + 3) return <>{text}</>;
    return (
        <span className={styles.middle} title={text}>
            <span className={styles.middleHead}>{text.slice(0, -TAIL_CHARACTERS)}</span>
            <span className={styles.middleTail}>{text.slice(-TAIL_CHARACTERS)}</span>
        </span>
    );
}

interface FieldRow {
    key: string;
    label: string;
    value: ReactNode;
}

/** Push a row only when its value exists — absent metadata renders nothing. */
function pushRow(rows: FieldRow[], key: string, label: string, value: ReactNode | null | undefined): void {
    if (value === null || value === undefined || value === "") return;
    rows.push({ key, label, value });
}

function Section({ head, rows, defaultExpanded }: { head: string; rows: FieldRow[]; defaultExpanded?: boolean }) {
    if (rows.length === 0) return null;
    return (
        <PanelSection head={head} intent="text" defaultExpanded={defaultExpanded}>
            {rows.map((row) => (
                <Row key={row.key} label={row.label} value={row.value} />
            ))}
        </PanelSection>
    );
}

/** The directory part of the source-relative path, or null for root files. */
function folderOf(relativePath: string): string | null {
    const lastSlash = relativePath.lastIndexOf("/");
    return lastSlash > 0 ? relativePath.slice(0, lastSlash) : null;
}

function JudgmentSection({ detail }: { detail: AssetDetail }) {
    const { t } = useTranslation();
    const rows: FieldRow[] = [{ key: "rating", label: t("inspector.rating"), value: <Rating value={detail.rating} /> }];
    if (detail.colorLabel !== null) {
        pushRow(
            rows,
            "colorLabel",
            t("inspector.colorLabel"),
            <span className={styles.labelValue}>
                <LabelSwatch label={detail.colorLabel} />
                {t(`colorLabel.${detail.colorLabel}`)}
            </span>,
        );
    }
    if (detail.flag !== null) {
        pushRow(
            rows,
            "flag",
            t("inspector.flag"),
            <span className={styles.labelValue}>
                <Icon concept={detail.flag === "reject" ? "reject" : "flag"} className={styles.flagGlyph} />
                {t(`flag.${detail.flag}`)}
            </span>,
        );
    }
    pushRow(rows, "note", t("inspector.note"), detail.note);
    return <Section head={t("inspector.sectionJudgment")} rows={rows} />;
}

function FileSection({ detail }: { detail: AssetDetail }) {
    const { t } = useTranslation();
    const rows: FieldRow[] = [];
    pushRow(rows, "filename", t("inspector.filename"), <MiddleTruncate text={detail.filename} />);
    pushRow(rows, "folder", t("inspector.folder"), folderOf(detail.relativePath));
    pushRow(rows, "size", t("inspector.size"), formatBytes(detail.sizeBytes));
    pushRow(rows, "fileType", t("inspector.fileType"), t(`fileType.${detail.fileType}`));
    pushRow(rows, "mime", t("inspector.mime"), detail.mimeType);
    // Normal is silent (§10): an online file shows no status row.
    if (detail.fileStatus !== "online") {
        pushRow(rows, "status", t("inspector.status"), t(`fileStatus.${detail.fileStatus}`));
    }
    if (detail.capturedAt !== null) {
        pushRow(rows, "captured", t("inspector.captured"), formatDateTime(detail.capturedAt));
    }
    pushRow(rows, "modified", t("inspector.modified"), formatDateTime(detail.mtime));
    pushRow(rows, "ingested", t("inspector.ingested"), formatDateTime(detail.ingestedAt));
    return <Section head={t("inspector.file")} rows={rows} />;
}

function CaptureSection({ detail }: { detail: AssetDetail }) {
    const { t } = useTranslation();
    const rows: FieldRow[] = [];
    if (detail.width !== null && detail.height !== null) {
        // Raw pixel counts, LrC-style — dimensions are never digit-grouped.
        pushRow(rows, "dimensions", t("inspector.dimensions"), `${detail.width} × ${detail.height}`);
    }
    if (detail.durationSecs !== null) {
        pushRow(rows, "duration", t("inspector.duration"), formatDuration(detail.durationSecs));
    }
    pushRow(rows, "exposure", t("inspector.exposure"), formatExposure(detail.shutterSpeed, detail.aperture));
    if (detail.iso !== null) {
        pushRow(rows, "iso", t("inspector.iso"), t("inspector.isoValue", { value: detail.iso }));
    }
    if (detail.focalLengthMm !== null) {
        pushRow(rows, "focalLength", t("inspector.focalLength"), formatFocalLength(detail.focalLengthMm));
    }
    pushRow(rows, "lens", t("inspector.lens"), detail.lensModel);
    if (detail.cameraMake !== null || detail.cameraModel !== null) {
        pushRow(rows, "camera", t("inspector.camera"), [detail.cameraMake, detail.cameraModel].filter(Boolean).join(" "));
    }
    pushRow(rows, "colorSpace", t("inspector.colorSpace"), detail.colorSpace);
    if (detail.bitDepth !== null) {
        pushRow(rows, "bitDepth", t("inspector.bitDepth"), t("inspector.bitDepthValue", { bits: detail.bitDepth }));
    }
    return <Section head={t("inspector.sectionCapture")} rows={rows} />;
}

function MetadataSection({ detail }: { detail: AssetDetail }) {
    const { t } = useTranslation();
    const rows: FieldRow[] = [];
    pushRow(rows, "title", t("inspector.title"), detail.title);
    pushRow(rows, "caption", t("inspector.caption"), detail.caption);
    pushRow(rows, "creator", t("inspector.creator"), detail.creator);
    pushRow(rows, "copyright", t("inspector.copyright"), detail.copyright);
    return <Section head={t("inspector.sectionMetadata")} rows={rows} />;
}

function LocationSection({ detail }: { detail: AssetDetail }) {
    const { t } = useTranslation();
    const rows: FieldRow[] = [];
    if (detail.gpsLat !== null && detail.gpsLon !== null) {
        pushRow(rows, "gps", t("inspector.gps"), formatGps(detail.gpsLat, detail.gpsLon));
    }
    return <Section head={t("inspector.location")} rows={rows} />;
}

/**
 * Extended-metadata values are untyped extraction output: mostly primitives,
 * but the engine ships structured values too (the importer's
 * `alexandria:extension_mismatch` is a map). Objects render as compact JSON —
 * honest data, never "[object Object]".
 */
function formatExtendedValue(value: unknown): string {
    if (value === null || typeof value !== "object") return String(value);
    return JSON.stringify(value);
}

function AllMetadataSection({ detail }: { detail: AssetDetail }) {
    const { t } = useTranslation();
    if (detail.extendedMetadata === undefined) return null;
    // The exiftool "Group:Tag" key IS the display vocabulary (decisions.md D24
    // dated note: standard, documented, ends naming debates) — data, not copy.
    const rows: FieldRow[] = Object.entries(detail.extendedMetadata)
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([key, value]) => ({ key, label: key, value: formatExtendedValue(value) }));
    return <Section head={t("inspector.sectionAll")} rows={rows} defaultExpanded={false} />;
}

export function Inspector() {
    const { t } = useTranslation();
    const cursorId = useCursorId();
    const { data, isError, isFetching, refetch } = useAsset(cursorId);

    if (cursorId === null) {
        return <StateNotice className={styles.empty}>{t("inspector.empty")}</StateNotice>;
    }
    // keepPreviousData holds the outgoing subject during navigation, so a bare
    // loading state only shows on the very first fetch of a session.
    if (data === undefined) {
        if (isError) {
            return (
                <div className={styles.stateColumn}>
                    <StateNotice>{t("inspector.error")}</StateNotice>
                    <Button rung="outline" onPress={() => void refetch()} isDisabled={isFetching}>
                        {t("inspector.retry")}
                    </Button>
                </div>
            );
        }
        return <StateNotice>{t("inspector.loading")}</StateNotice>;
    }

    return (
        <div className={styles.panel}>
            <JudgmentSection detail={data} />
            <FileSection detail={data} />
            <CaptureSection detail={data} />
            <MetadataSection detail={data} />
            <LocationSection detail={data} />
            <AllMetadataSection detail={data} />
        </div>
    );
}

function StateNotice({ children, className }: { children: ReactNode; className?: string }) {
    return <div className={cx(styles.state, className)}>{children}</div>;
}
