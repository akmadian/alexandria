// TextField — the field composite (frontend/CLAUDE.md §6): label + input well
// + description + error on the classic RAC TextField. The well recesses (§7:
// a field you can type in is a slightly darker well); invalid is the §5
// ERROR-RED hairline with the message row in INK — error red is its own ledger
// row, independent of the attention hue, and the message is never red text.

import {
    FieldError as AriaFieldError,
    Input as AriaInput,
    Label as AriaLabel,
    Text as AriaText,
    TextField as AriaTextField,
    type TextFieldProps as AriaTextFieldProps,
} from "react-aria-components";
import { cx } from "@/lib/cx";
import styles from "./text-field.module.css";

export type TextFieldSize = "xs" | "sm" | "md" | "lg";

// C10: exhaustive by construction, mirroring Button.
const SIZE_CLASSES = {
    xs: styles.controlXsmall,
    sm: styles.controlSmall,
    md: styles.controlMedium,
    lg: styles.controlLarge,
} as const satisfies Record<TextFieldSize, string>;

export interface TextFieldProps
    extends Omit<AriaTextFieldProps, "children" | "className" | "style"> {
    /** The field's label, in the label role. */
    label: string;
    /** Hint text under the input, in the caption role. */
    description?: string;
    /** Shown only while invalid — the §25 message row, in ink. */
    errorMessage?: string;
    placeholder?: string;
    /** §8 size ladder: xs = 16px (inspector inline-edit — matches the read-only row),
     * sm = 20px (dense inline-edit), md = 24px (the dense-tool default), lg = 28px. */
    size?: TextFieldSize;
    className?: string;
}

export function TextField({
    label,
    description,
    errorMessage,
    placeholder,
    size = "md",
    className,
    ...ariaProps
}: TextFieldProps) {
    return (
        <AriaTextField {...ariaProps} className={cx(styles.field, className)}>
            <AriaLabel className={styles.label}>{label}</AriaLabel>
            <AriaInput placeholder={placeholder} className={cx(styles.input, SIZE_CLASSES[size])} />
            {description !== undefined && (
                <AriaText slot="description" className={styles.description}>
                    {description}
                </AriaText>
            )}
            <AriaFieldError className={styles.error}>{errorMessage}</AriaFieldError>
        </AriaTextField>
    );
}
