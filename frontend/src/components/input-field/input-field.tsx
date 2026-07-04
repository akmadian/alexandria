import {
    FieldError as AriaFieldError,
    Input as AriaInput,
    Label as AriaLabel,
    TextArea as AriaTextArea,
    TextField as AriaTextField,
    type TextFieldProps,
} from "react-aria-components";
import { cx } from "@/lib/cx";
import s from "./input-field.module.css";

interface InputFieldProps extends Omit<TextFieldProps, "className"> {
    label?: string;
    placeholder?: string;
    errorMessage?: string;
    multiline?: boolean;
    rows?: number;
    className?: string;
}

export const InputField = ({ label, placeholder, errorMessage, multiline, rows = 3, className, ...rest }: InputFieldProps) => (
    <AriaTextField {...rest} className={cx(s.field, className)}>
        {label && <AriaLabel className="u-label">{label}</AriaLabel>}
        {multiline ? <AriaTextArea className={s.input} placeholder={placeholder} rows={rows} /> : <AriaInput className={s.input} placeholder={placeholder} />}
        <AriaFieldError className={s.error}>{errorMessage}</AriaFieldError>
    </AriaTextField>
);
