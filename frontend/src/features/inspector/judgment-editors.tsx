// The inspector's Judgment editors (task 34). The read-only Rating / label swatch
// / flag / note become interactive IN PLACE — same marks, now writable. The
// editors are target-agnostic (they emit patches through the `write` prop); the
// TARGET is the C5 selection-else-cursor, resolved by the inspector's composition
// (Ari ruling 2026-07-20 — the panel is a verb surface like the triage keys).
// Displayed values are the subject's (the cursor asset); a mixed-value display
// for differing selections is the deferred refinement the read-only round
// flagged. Grid cell faces stay read-only (cell-face editing is a later round).
// Feature-local; promotes to components/ when a second surface edits triage.

import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { ColorLabel, Flag } from "@/_generated-types/enums";
import type { TriagePatch } from "@/api/contract";
import { Icon } from "@/components/icon/icon";
import { LabelSwatch } from "@/components/label-swatch/label-swatch";
import { Rating } from "@/components/rating/rating";
import { TextField } from "@/components/text-field/text-field";
import { ToggleButton } from "@/components/toggle-button/toggle-button";
import styles from "./inspector.module.css";

// The five assignable labels (§5) — orange stays in the enum for XMP round-trip
// but is not offered (dropped from the palette 2026-07-18).
const ASSIGNABLE_LABELS: readonly ColorLabel[] = ["red", "yellow", "green", "blue", "purple"];

export type WriteTriage = (patch: TriagePatch) => void;

export function RatingEditor({ value, write }: { value: number | null; write: WriteTriage }) {
    // Rating already encodes the gesture grammar (star n → n; the current value →
    // null clear), matching the keyboard's "0 clears".
    return <Rating value={value} onChange={(next) => write({ rating: next })} />;
}

export function LabelEditor({ value, write }: { value: ColorLabel | null; write: WriteTriage }) {
    const { t } = useTranslation();
    // Toggle-to-clear (LrC): clicking the active label clears it; clicking another
    // sets it. The register grammar (rest/hover/pressed/selected fill, focus ring,
    // aria-pressed) is the ToggleButton primitive's — this composes it and
    // overrides only the swatch-pad geometry. State is never color alone (§10):
    // the label's name is the aria-label, the selected fill is the state.
    return (
        <span role="group" aria-label={t("inspector.colorLabel")} className={styles.editorGroup}>
            {ASSIGNABLE_LABELS.map((label) => {
                const active = value === label;
                return (
                    <ToggleButton
                        key={label}
                        isSelected={active}
                        aria-label={t(`colorLabel.${label}`)}
                        className={styles.judgmentToggle}
                        onChange={() => write({ colorLabel: active ? null : label })}
                    >
                        <LabelSwatch label={label} />
                    </ToggleButton>
                );
            })}
        </span>
    );
}

export function FlagEditor({ value, write }: { value: Flag | null; write: WriteTriage }) {
    const { t } = useTranslation();
    // Each flag toggles to clear; pick and reject are mutually exclusive (setting
    // one replaces the other — an absolute value, no separate clear needed).
    const set = (flag: Flag) => write({ flag: value === flag ? null : flag });
    return (
        <span role="group" aria-label={t("inspector.flag")} className={styles.editorGroup}>
            <ToggleButton
                isSelected={value === "pick"}
                aria-label={t("actions.flag_pick")}
                className={styles.judgmentToggle}
                onChange={() => set("pick")}
            >
                <Icon concept="flag" />
            </ToggleButton>
            <ToggleButton
                isSelected={value === "reject"}
                aria-label={t("actions.flag_reject")}
                className={styles.judgmentToggle}
                onChange={() => set("reject")}
            >
                <Icon concept="reject" />
            </ToggleButton>
        </span>
    );
}

export function NoteEditor({ value, write }: { value: string | null; write: WriteTriage }) {
    const { t } = useTranslation();
    // Local state owns the input while editing; the write commits on blur, so a
    // note is one write, not one per keystroke. The parent keys this by asset id,
    // so switching subject remounts and reseeds from the new detail.
    const [draft, setDraft] = useState(value ?? "");
    const commit = () => {
        if (draft === (value ?? "")) return;
        // Empty clears (null); the three-state patch carries the clear explicitly.
        write({ note: draft === "" ? null : draft });
    };
    return (
        <TextField
            label={t("inspector.note")}
            placeholder={t("inspector.notePlaceholder")}
            value={draft}
            onChange={setDraft}
            onBlur={commit}
            className={styles.noteField}
        />
    );
}
