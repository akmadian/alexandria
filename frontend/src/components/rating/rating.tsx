// Rating — the five-position star readout/input (§14: icons are ink, fill = on).
// Controlled and stateless: `value` down; the optional `onChange` reports the
// NEXT rating with the gesture grammar encoded once — clicking star n proposes
// n, clicking the current value proposes null (the LrC clear; the keyboard's
// "0 clears" maps to the same event when the actions round wires it). No RAC:
// a radiogroup captures arrow keys, which content surfaces reserve for grid
// navigation — display mode (no `onChange`) renders zero tab stops for exactly
// that reason. Silent-vs-shown is the CONSUMER's call (§10): this component
// always renders five positions for whatever value it is handed.

import { useTranslation } from "react-i18next";
import { Icon } from "@/components/icon/icon";
import { cx } from "@/lib/cx";
import styles from "./rating.module.css";

const STAR_POSITIONS = [1, 2, 3, 4, 5] as const;

export interface RatingProps {
    /** 1–5, or null = unrated. A defensive 0 renders like null (five empty
     * positions) — the contract's truth is that 0 is not a rating. */
    value: number | null;
    /** Present = interactive: five buttons reporting the next rating (null = clear). */
    onChange?: (next: number | null) => void;
    className?: string;
}

export function Rating({ value, onChange, className }: RatingProps) {
    const { t } = useTranslation();
    const filled = value ?? 0;
    const stateLabel = filled > 0 ? t("rating.rated", { value: filled }) : t("rating.unrated");

    if (onChange === undefined) {
        return (
            <span className={cx(styles.rating, className)} aria-label={stateLabel}>
                {STAR_POSITIONS.map((position) => (
                    <Icon
                        key={position}
                        concept="rating"
                        className={position <= filled ? styles.on : styles.off}
                    />
                ))}
            </span>
        );
    }

    return (
        <span role="group" className={cx(styles.rating, styles.interactive, className)} aria-label={stateLabel}>
            {STAR_POSITIONS.map((position) => (
                <button
                    key={position}
                    type="button"
                    className={styles.starButton}
                    aria-label={position === filled ? t("actions.rate_0") : t(`actions.rate_${String(position)}`)}
                    onClick={() => onChange(position === filled ? null : position)}
                >
                    <Icon concept="rating" className={position <= filled ? styles.on : styles.off} />
                </button>
            ))}
        </span>
    );
}
