import type { ColorLabel, FileStatus, FileType, Flag } from "./enums.ts";

export interface Asset {
    id: string;
    sourceId: string;
    relativePath: string; // path within the source, '/'-separated, includes filename
    filename: string;
    extension: string;
    fileType: FileType;
    fileStatus: FileStatus;
    sizeBytes: number;
    width: number;
    height: number;
    durationSecs?: number; // temporal media only
    capturedAt: string; // ISO
    cameraMake?: string;
    cameraModel?: string;
    lensModel?: string;
    focalLengthMM?: number;
    aperture?: number;
    shutterSpeed?: string;
    iso?: number;
    location?: string;
    creator?: string;
    rating: number; // 0-5
    colorLabel: ColorLabel | null;
    flag: Flag;
    note: string | null;
    tagIds: string[];
    ingestedAt: string; // ISO
    thumbnailAt: string; // ISO — content-address for the versioned thumbnail URL
}
