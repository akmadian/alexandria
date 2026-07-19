export namespace ast {
	
	export interface Arrangement {
	    SortField: string;
	    SortDir: string;
	}
	export interface Page {
	    Limit: number;
	    Offset: number;
	}
	export interface Scope {
	    Kind: string;
	    ID: string;
	    SourceID: string;
	    Path: string;
	    Recursive: boolean;
	}
	export interface Query {
	    Version: number;
	    Scope?: Scope;
	    Where: any;
	}

}

export namespace catalog {
	
	export interface AssetRow {
	    id: string;
	    sourceId: string;
	    filename: string;
	    fileType: string;
	    fileStatus: string;
	    rating?: number;
	    colorLabel?: string;
	    flag?: string;
	    width?: number;
	    height?: number;
	    durationSecs?: number;
	    cameraModel?: string;
	    // Go type: time
	    capturedAt?: any;
	    // Go type: time
	    ingestedAt: any;
	    // Go type: time
	    thumbnailAt?: any;
	    relativePath: string;
	    sizeBytes: number;
	    enriching?: string[];
	    failed?: string[];
	}

}

export namespace domain {
	
	export interface Asset {
	    ID: string;
	    SourceID: string;
	    RelativePath: string;
	    FileStatus: string;
	    // Go type: time
	    LastVerifiedAt?: any;
	    Filename: string;
	    Extension: string;
	    MIMEType: string;
	    FileType: string;
	    SizeBytes: number;
	    // Go type: time
	    MTime: any;
	    PartialHash: string;
	    Width?: number;
	    Height?: number;
	    DurationSecs?: number;
	    ColorSpace?: string;
	    BitDepth?: number;
	    // Go type: time
	    CapturedAt?: any;
	    CameraMake?: string;
	    CameraModel?: string;
	    LensModel?: string;
	    FocalLengthMM?: number;
	    Aperture?: number;
	    ShutterSpeed?: string;
	    ISO?: number;
	    GPSLat?: number;
	    GPSLon?: number;
	    Creator?: string;
	    Copyright?: string;
	    Title?: string;
	    Caption?: string;
	    ExtendedMetadata: Record<string, any>;
	    Rating?: number;
	    ColorLabel?: string;
	    Flag?: string;
	    Note?: string;
	    // Go type: time
	    JudgmentModifiedAt?: any;
	    // Go type: time
	    XMPLastReadAt?: any;
	    // Go type: time
	    XMPLastWrittenAt?: any;
	    XMPHash?: string;
	    // Go type: time
	    ThumbnailAt?: any;
	    Sharpness?: number;
	    ClippingHighlights?: number;
	    ClippingShadows?: number;
	    IsDeleted: boolean;
	    // Go type: time
	    DeletedAt?: any;
	    // Go type: time
	    IngestedAt: any;
	    // Go type: time
	    UpdatedAt: any;
	}
	export interface Collection {
	    ID: string;
	    Name: string;
	    ParentID?: string;
	    Kind: string;
	    Query?: string;
	    CoverAssetID?: string;
	    SortField?: string;
	    SortDir: string;
	    // Go type: time
	    CreatedAt: any;
	    // Go type: time
	    UpdatedAt: any;
	}
	export interface Source {
	    ID: string;
	    Name: string;
	    Kind: string;
	    BasePath: string;
	    FilesystemUUID?: string;
	    DiskSerial?: string;
	    VolumeLabel?: string;
	    Host?: string;
	    ShareName?: string;
	    PollIntervalSecs?: number;
	    ScanRecursively: boolean;
	    Enabled: boolean;
	    Connectivity: string;
	    // Go type: time
	    LastScannedAt?: any;
	    // Go type: time
	    CreatedAt: any;
	    // Go type: time
	    UpdatedAt: any;
	}

}

export namespace seam {
	
	export interface CollectionInput {
	    name: string;
	    parentId?: string;
	    kind?: string;
	    query?: ast.Query;
	}
	export interface CollectionPatch {
	    name?: string;
	    coverAssetId?: string;
	    query?: ast.Query;
	}
	export interface QueryResult {
	    items: catalog.AssetRow[];
	    total: number;
	}
	export interface SourceInput {
	    name: string;
	    kind?: string;
	    basePath: string;
	    scanRecursively?: boolean;
	}
	export interface SourcePatch {
	    name?: string;
	    enabled?: boolean;
	    scanRecursively?: boolean;
	}
	export interface TriagePatchInput {
	    rating?: number[];
	    colorLabel?: number[];
	    flag?: number[];
	    note?: number[];
	}
	export interface UpdateTarget {
	    ids?: string[];
	    query?: ast.Query;
	    exceptIds?: string[];
	}

}

export namespace settings {
	
	export interface Settings {
	    thumbnailQuality: number;
	    importBatchSize: number;
	    ignorePatterns: string[];
	    xmpWriteBack: boolean;
	    xmpConflictResolution: string;
	    ui?: number[];
	}

}

