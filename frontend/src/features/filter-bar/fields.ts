// The fields the filter bar offers, grouped for the picker — feature-local domain
// context (frontend/09 "features = primitives + how the user uses them"). Each
// field's `kind` is pulled from the GENERATED fieldGrammar so it can't drift from
// the token contract (C13); the bar adds only category + (for enums) the runtime
// member list, which the types-only generated enums don't carry (see below).

import type { ColorLabel, FileType, Flag } from "@/_generated-types/enums";
import { type TokenField, type ValueKind, fieldGrammar } from "@/_generated-types/vocabulary";

// Runtime enum members via the completeness trick — a Record keyed by every union
// member only compiles when exhaustive, so a newly generated value breaks the build
// here until handled (C10/C13). ponytail: fold into a generated `enumValues` map if
// the seam generator grows one.
const membersOf = <T extends string>(present: Record<T, true>): readonly T[] => Object.keys(present) as T[];
const fileTypeMembers = membersOf<FileType>({ image: true, raw: true, video: true, vector: true, document: true, audio: true });
const flagMembers = membersOf<Flag>({ pick: true, reject: true });
const colorLabelMembers = membersOf<ColorLabel>({ red: true, orange: true, yellow: true, green: true, blue: true, purple: true });

export type FilterCategory = "triage" | "file" | "capture" | "metadata";
export const CATEGORY_ORDER: readonly FilterCategory[] = ["triage", "file", "capture", "metadata"];

export interface FilterField {
    field: TokenField;
    kind: ValueKind; // mirrored from the generated grammar — drives the value editor
    category: FilterCategory;
    members?: readonly string[]; // enum kinds only
    labelNs?: string; // enum kinds: i18n namespace for member labels (`<labelNs>.<member>`)
}

const field = (field: TokenField, category: FilterCategory, extra?: Partial<FilterField>): FilterField => ({
    field,
    kind: fieldGrammar[field].kind,
    category,
    ...extra,
});

// Picker order within each category. Only fields whose kind has a value editor
// (enum/numeric/text today) are offered; date/tag/source join as their editors land.
export const FILTER_FIELDS: readonly FilterField[] = [
    field("rating", "triage"),
    field("flag", "triage", { members: flagMembers, labelNs: "flag" }),
    field("colorLabel", "triage", { members: colorLabelMembers, labelNs: "colorLabel" }),
    field("fileType", "file", { members: fileTypeMembers, labelNs: "fileType" }),
    field("filename", "file"),
    field("cameraMake", "capture"),
    field("cameraModel", "capture"),
    field("title", "metadata"),
];

const BY_FIELD: Partial<Record<TokenField, FilterField>> = Object.fromEntries(FILTER_FIELDS.map((f) => [f.field, f]));

/** The offered field's definition, or undefined if the bar doesn't handle it yet. */
export function filterFieldFor(field: TokenField): FilterField | undefined {
    return BY_FIELD[field];
}
