// Read-only metadata display. A dumb table of key/value rows over Asset slices;
// each row hidden when its datum is absent. New inspector fields = new rows here
// (seam doc change playbook: "show another metadata field" touches only this).

import { useTranslation } from "react-i18next";
import { formatBytes, formatDateTime } from "@/lib/format";
import type { Asset } from "@/api/contract";
import s from "./meta-section.module.css";

const Row = ({ label, value }: { label: string; value?: string | number | null }) =>
    value == null || value === "" ? null : (
        <div className={s.row}>
            <dt className="u-label">{label}</dt>
            <dd className={`${s.value} u-data`}>{value}</dd>
        </div>
    );

export const MetaSection = ({ asset }: { asset: Asset }) => {
    const { t } = useTranslation();
    const exposure = [asset.aperture && `ƒ/${asset.aperture}`, asset.shutterSpeed, asset.iso && `ISO ${asset.iso}`, asset.focalLengthMM && `${asset.focalLengthMM}mm`]
        .filter(Boolean)
        .join(" · ");
    return (
        <dl className={s.meta}>
            <Row label={t("inspector.file")} value={asset.filename} />
            <Row label={t("inspector.captured")} value={formatDateTime(asset.capturedAt)} />
            <Row label={t("inspector.dimensions")} value={`${asset.width} × ${asset.height}`} />
            <Row label={t("inspector.size")} value={formatBytes(asset.sizeBytes)} />
            <Row label={t("inspector.camera")} value={[asset.cameraMake, asset.cameraModel].filter(Boolean).join(" ")} />
            <Row label={t("inspector.lens")} value={asset.lensModel} />
            <Row label={t("inspector.exposure")} value={exposure} />
            <Row label={t("inspector.location")} value={asset.location} />
        </dl>
    );
};
