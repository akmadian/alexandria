// The models barrel — the single door for domain shapes. api/contract.ts
// re-exports these as part of the contract, so app code imports them from
// `@/api/contract`, not from here.
//
// When the Wails backend binds, these hand-written shapes get replaced by types
// generated from Go `internal/domain`; this barrel is where that swap lands.

export type { FileType, ColorLabel, Flag, FileStatus, SourceStatus } from "./enums.ts";
export type { Asset } from "./asset.ts";
export type { Source } from "./source.ts";
export type { Collection } from "./collection.ts";
export type { Tag } from "./tag.ts";
