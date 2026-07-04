/** Join class names, skipping falsy values. The entire styling utility layer. */
export const cx = (...parts: (string | false | null | undefined)[]): string => parts.filter(Boolean).join(" ");
