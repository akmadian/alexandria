// Mock data modeled on internal/domain (Go). No backend yet — thumbnails use
// picsum so the grid reads like a real photo library.
// ponytail: static mock; swap for the Wails-bound catalog API when it exists.

export type FileType = "image" | "video" | "raw" | "vector" | "document" | "audio";
export type ColorLabel = "red" | "orange" | "yellow" | "green" | "blue" | "purple";
export type Flag = "pick" | "reject" | null;
export type FileStatus = "online" | "offline" | "missing";
export type SourceStatus = "active" | "offline";

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

export interface Source {
    id: string;
    name: string;
    kind: "local" | "external_drive" | "smb" | "nfs";
    status: SourceStatus;
    count: number;
}

export interface Collection {
    id: string;
    name: string;
    kind: "manual" | "smart";
    count: number;
}

export interface Tag {
    id: string;
    name: string;
    color: ColorLabel;
    count: number;
}

export const sources: Source[] = [
    { id: "src-main", name: "Main SSD", kind: "local", status: "active", count: 18 },
    { id: "src-photos", name: "Photo Archive", kind: "external_drive", status: "active", count: 6 },
    { id: "src-nas", name: "Studio NAS", kind: "smb", status: "offline", count: 0 },
];

export const collections: Collection[] = [
    { id: "col-portfolio", name: "Portfolio 2026", kind: "manual", count: 12 },
    { id: "col-clients", name: "Client Delivery", kind: "manual", count: 8 },
    { id: "col-5star", name: "Five Star Picks", kind: "smart", count: 5 },
    { id: "col-untagged", name: "Needs Tagging", kind: "smart", count: 7 },
];

export const tags: Tag[] = [
    { id: "t-landscape", name: "Landscape", color: "green", count: 9 },
    { id: "t-portrait", name: "Portrait", color: "purple", count: 6 },
    { id: "t-street", name: "Street", color: "orange", count: 5 },
    { id: "t-clientwork", name: "Client Work", color: "blue", count: 8 },
    { id: "t-archive", name: "Archive", color: "red", count: 4 },
];

const cameras: [string, string, string][] = [
    ["Sony", "α7 IV", "FE 24-70mm F2.8 GM II"],
    ["Canon", "EOS R5", "RF 50mm F1.2 L"],
    ["Nikon", "Z8", "NIKKOR Z 70-200mm f/2.8"],
    ["Fujifilm", "X-T5", "XF 35mm F1.4 R"],
    ["Leica", "Q3", "Summilux 28mm f/1.7"],
];

const shots: { name: string; type: FileType; loc?: string; w: number; h: number }[] = [
    { name: "Golden hour ridge", type: "raw", loc: "Dolomites, IT", w: 6000, h: 4000 },
    { name: "Studio headshot — Mara", type: "raw", loc: "Studio A", w: 4000, h: 6000 },
    { name: "Neon crosswalk", type: "image", loc: "Shibuya, JP", w: 6000, h: 4000 },
    { name: "Coastal fog", type: "image", loc: "Big Sur, US", w: 6000, h: 4000 },
    { name: "Product — ceramic mug", type: "image", loc: "Studio B", w: 5000, h: 5000 },
    { name: "Forest path", type: "raw", loc: "Black Forest, DE", w: 6000, h: 4000 },
    { name: "Rooftop portrait", type: "image", loc: "Brooklyn, US", w: 4000, h: 6000 },
    { name: "Desert dunes", type: "raw", loc: "Merzouga, MA", w: 6000, h: 4000 },
    { name: "Campaign cut v3", type: "video", loc: "Studio A", w: 3840, h: 2160 },
    { name: "Market vendor", type: "image", loc: "Marrakech, MA", w: 6000, h: 4000 },
    { name: "Mountain lake", type: "raw", loc: "Banff, CA", w: 6000, h: 4000 },
    { name: "Editorial — knitwear", type: "image", loc: "Loft 5", w: 4000, h: 6000 },
    { name: "Rain on glass", type: "image", loc: "London, UK", w: 6000, h: 4000 },
    { name: "Vineyard rows", type: "raw", loc: "Tuscany, IT", w: 6000, h: 4000 },
    { name: "Skater midair", type: "image", loc: "Venice Beach, US", w: 6000, h: 4000 },
    { name: "Brand logo lockup", type: "vector", w: 2400, h: 2400 },
    { name: "Snow peak alpenglow", type: "raw", loc: "Chamonix, FR", w: 6000, h: 4000 },
    { name: "Cafe window seat", type: "image", loc: "Paris, FR", w: 6000, h: 4000 },
    { name: "Behind the scenes", type: "video", loc: "Studio A", w: 3840, h: 2160 },
    { name: "Wildflower macro", type: "image", loc: "Provence, FR", w: 6000, h: 4000 },
    { name: "Old town alley", type: "image", loc: "Lisbon, PT", w: 4000, h: 6000 },
    { name: "Tide pools", type: "raw", loc: "Oregon Coast, US", w: 6000, h: 4000 },
    { name: "Concert crowd", type: "image", loc: "Berlin, DE", w: 6000, h: 4000 },
    { name: "Contract & brief", type: "document", w: 1700, h: 2200 },
];

const labels: (ColorLabel | null)[] = ["red", "yellow", "green", "blue", "purple", null, null, null];
const flags: Flag[] = ["pick", null, null, "pick", "reject", null, "pick", null];
const extFor: Record<FileType, string> = {
    raw: "ARW", image: "JPG", video: "MP4", vector: "SVG", document: "PDF", audio: "WAV",
};

// Folder buckets per source, so the derived filesystem tree has real structure to show.
const foldersBySource: Record<string, string[]> = {
    "src-main": ["2026/06/Dolomites", "2026/06/Studio", "2026/05/Japan", "Clients/Acme", "Personal/Travel"],
    "src-photos": ["Archive/2025/Landscapes", "Archive/2025/Portraits", "Archive/2024"],
};

export const assets: Asset[] = shots.map((s, i) => {
    const [make, model, lens] = cameras[i % cameras.length];
    const hasExif = s.type === "raw" || s.type === "image";
    const day = String((i % 27) + 1).padStart(2, "0");
    const status: FileStatus = i % 11 === 0 ? "offline" : "online";
    const sourceId = i < 18 ? "src-main" : "src-photos";
    const filename = `${s.name.replace(/[^a-z0-9]+/gi, "_").replace(/^_|_$/g, "")}.${extFor[s.type].toLowerCase()}`;
    const folders = foldersBySource[sourceId];
    const ingestedAt = `2026-06-${String((i % 27) + 1).padStart(2, "0")}T09:${String((i * 13) % 60).padStart(2, "0")}:00Z`;
    return {
        id: `asset-${String(i + 1).padStart(3, "0")}`,
        sourceId,
        relativePath: `${folders[i % folders.length]}/${filename}`,
        filename,
        extension: extFor[s.type],
        fileType: s.type,
        fileStatus: status,
        sizeBytes: (s.type === "raw" ? 42 : s.type === "video" ? 380 : 8) * 1024 * 1024 + i * 111111,
        width: s.w,
        height: s.h,
        durationSecs: s.type === "video" ? 8 + (i % 5) * 6.5 : undefined,
        capturedAt: `2026-0${(i % 6) + 1}-${day}T${String(8 + (i % 10)).padStart(2, "0")}:${String((i * 7) % 60).padStart(2, "0")}:00Z`,
        cameraMake: hasExif ? make : undefined,
        cameraModel: hasExif ? model : undefined,
        lensModel: hasExif ? lens : undefined,
        focalLengthMM: hasExif ? [24, 35, 50, 85, 135, 200][i % 6] : undefined,
        aperture: hasExif ? [1.4, 1.8, 2.8, 4, 5.6, 8][i % 6] : undefined,
        shutterSpeed: hasExif ? ["1/1000", "1/500", "1/250", "1/125", "1/60"][i % 5] : undefined,
        iso: hasExif ? [100, 200, 400, 800, 1600][i % 5] : undefined,
        location: s.loc,
        creator: "Ari Madian",
        rating: [5, 4, 3, 0, 5, 2, 4, 0][i % 8],
        colorLabel: labels[i % labels.length],
        flag: flags[i % flags.length],
        note: i % 4 === 0 ? "Flagged for portfolio review." : null,
        tagIds: tags.filter((_, ti) => (i + ti) % 3 === 0).map((t) => t.id).slice(0, 3),
        ingestedAt,
        thumbnailAt: ingestedAt,
    };
});

// Thumbnail URL — picsum seeded by id, aspect from the asset dimensions.
export function thumbUrl(a: Asset, w = 500): string {
    const h = Math.round((w * a.height) / a.width);
    return `https://picsum.photos/seed/${a.id}/${w}/${h}`;
}

export function formatBytes(n: number): string {
    if (n < 1024) return `${n} B`;
    const u = ["KB", "MB", "GB", "TB"];
    let v = n / 1024;
    let i = 0;
    while (v >= 1024 && i < u.length - 1) {
        v /= 1024;
        i++;
    }
    return `${v.toFixed(1)} ${u[i]}`;
}

export function formatDate(iso: string): string {
    return new Date(iso).toLocaleDateString("en-US", { year: "numeric", month: "short", day: "numeric" });
}

export function formatDateTime(iso: string): string {
    return new Date(iso).toLocaleString("en-US", {
        year: "numeric", month: "short", day: "numeric", hour: "numeric", minute: "2-digit",
    });
}

export const colorLabelClass: Record<ColorLabel, string> = {
    red: "bg-red-500",
    orange: "bg-orange-500",
    yellow: "bg-yellow-400",
    green: "bg-green-500",
    blue: "bg-blue-500",
    purple: "bg-purple-500",
};
