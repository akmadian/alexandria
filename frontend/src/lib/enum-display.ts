// The single place seam convention 6 lives: enum codes → { icon, labelKey },
// with a mandatory fallback so an unknown value from a newer backend degrades
// to a generic rendering instead of crashing the grid. Components never switch
// on enum codes themselves.

import { Aperture, File, FileText, HardDrive, Image, Music, PenTool, Server, Usb, Video, type LucideIcon } from "lucide-react";
import type { FileType, Source } from "@/api/contract";

interface EnumDisplay {
    icon: LucideIcon;
    labelKey: string; // i18n key — display text is always translated, never hardcoded
}

const FILE_TYPES: Record<FileType, EnumDisplay> = {
    image: { icon: Image, labelKey: "fileType.image" },
    raw: { icon: Aperture, labelKey: "fileType.raw" },
    video: { icon: Video, labelKey: "fileType.video" },
    vector: { icon: PenTool, labelKey: "fileType.vector" },
    document: { icon: FileText, labelKey: "fileType.document" },
    audio: { icon: Music, labelKey: "fileType.audio" },
};

const FILE_TYPE_FALLBACK: EnumDisplay = { icon: File, labelKey: "fileType.unknown" };

export function fileTypeDisplay(t: FileType | string): EnumDisplay {
    return FILE_TYPES[t as FileType] ?? FILE_TYPE_FALLBACK;
}

const SOURCE_KINDS: Record<Source["kind"], EnumDisplay> = {
    local: { icon: HardDrive, labelKey: "sourceKind.local" },
    external_drive: { icon: Usb, labelKey: "sourceKind.external_drive" },
    smb: { icon: Server, labelKey: "sourceKind.smb" },
    nfs: { icon: Server, labelKey: "sourceKind.nfs" },
};

const SOURCE_KIND_FALLBACK: EnumDisplay = { icon: HardDrive, labelKey: "sourceKind.unknown" };

export function sourceKindDisplay(k: Source["kind"] | string): EnumDisplay {
    return SOURCE_KINDS[k as Source["kind"]] ?? SOURCE_KIND_FALLBACK;
}
