import type { SourceStatus } from "./enums.ts";

export interface Source {
    id: string;
    name: string;
    kind: "local" | "external_drive" | "smb" | "nfs";
    status: SourceStatus;
    count: number;
}
