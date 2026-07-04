// Shared scalar vocabulary — the enum-like string unions used across entities.
// Mirrors the enums in Go `internal/domain`.

export type FileType = "image" | "video" | "raw" | "vector" | "document" | "audio";
export type ColorLabel = "red" | "orange" | "yellow" | "green" | "blue" | "purple";
export type Flag = "pick" | "reject" | null;
export type FileStatus = "online" | "offline" | "missing";
export type SourceStatus = "active" | "offline";
