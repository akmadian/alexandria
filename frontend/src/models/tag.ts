import type { ColorLabel } from "./enums.ts";

export interface Tag {
    id: string;
    name: string;
    color: ColorLabel;
    count: number;
}
