// Right pane: full record of the current subject (last-selected asset) plus the
// triage edit controls. Reads server state via useAsset; writes via the
// optimistic usePatchAssets / useSetAssetTags. Metadata sections are dumb
// subcomponents taking Asset slices. Multi-select mixed-value editing is
// designed-not-built (seam doc §15.2) — today it inspects the last selection.

import { useTranslation } from "react-i18next";
import { Button } from "@/components/button/button";
import { TagChip } from "@/components/tag-chip/tag-chip";
import { useLibraryState } from "@/app/library-state";
import { useAsset, usePatchAssets, useTags } from "@/api/queries";
import type { Asset, ColorLabel, Flag } from "@/api/contract";
import { MetaSection } from "./meta-section";
import { RatingControl } from "./rating-control";
import s from "./inspector-view.module.css";

const LABELS: ColorLabel[] = ["red", "orange", "yellow", "green", "blue", "purple"];

export const InspectorView = () => {
    const { t } = useTranslation();
    const { lastSelectedId, selection } = useLibraryState();
    const { data: asset } = useAsset(lastSelectedId);
    const allTags = useTags().data ?? [];
    const patchAssets = usePatchAssets();

    if (!asset) return <div className={s.empty}>{t("inspector.empty")}</div>;

    // Edits apply to the whole selection if multiple are selected, else the subject.
    const targetIds = selection.size > 1 ? [...selection] : [asset.id];
    const patch = (p: Parameters<typeof patchAssets.mutate>[0]["patch"]) => patchAssets.mutate({ target: { ids: targetIds }, patch: p });

    const assetTags = asset.tagIds.map((id) => allTags.find((tg) => tg.id === id)).filter(Boolean);

    return (
        <div className={s.inspector}>
            <div className={s.triage}>
                <label className="u-label">{t("inspector.rating")}</label>
                <RatingControl value={asset.rating} onChange={(rating) => patch({ rating: rating === 0 ? null : rating })} />

                <label className="u-label">{t("inspector.flag")}</label>
                <div className={s.flags}>
                    {(["pick", "reject"] as Exclude<Flag, null>[]).map((f) => (
                        <Button key={f} size="sm" variant={asset.flag === f ? "primary" : "ghost"} onPress={() => patch({ flag: asset.flag === f ? null : f })}>
                            {t(`flag.${f}`)}
                        </Button>
                    ))}
                </div>

                <label className="u-label">{t("inspector.colorLabel")}</label>
                <div className={s.labels}>
                    {LABELS.map((c) => (
                        <button
                            key={c}
                            className={s.swatch}
                            data-active={asset.colorLabel === c || undefined}
                            style={{ background: `var(--label-${c})` }}
                            aria-label={t(`colorLabel.${c}`)}
                            onClick={() => patch({ colorLabel: asset.colorLabel === c ? null : c })}
                        />
                    ))}
                </div>
            </div>

            {assetTags.length > 0 && (
                <div className={s.tags}>
                    <label className="u-label">{t("inspector.tags")}</label>
                    <div className={s.tagRow}>
                        {assetTags.map((tg) => tg && <TagChip key={tg.id} name={tg.name} color={tg.color} />)}
                    </div>
                </div>
            )}

            <MetaSection asset={asset} />
        </div>
    );
};

export type { Asset };
