export namespace domain {
	
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

