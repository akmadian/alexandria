package volume

import (
	"strings"
	"testing"
)

// Internal-package tests for the pure probe parsers (§1: bytes in, facts out —
// each parser is exercised on realistic fixture bytes plus a malformed case).

// diskutilFixture is a trimmed real `diskutil info -plist` record: the keys the
// parser consumes plus representative noise (nested containers, unrelated
// scalars) that it must skip.
const diskutilFixture = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>BusProtocol</key>
	<string>Apple Fabric</string>
	<key>CanBeMadeBootable</key>
	<false/>
	<key>Ejectable</key>
	<false/>
	<key>Internal</key>
	<true/>
	<key>MediaName</key>
	<string></string>
	<key>APFSPhysicalStores</key>
	<array>
		<dict>
			<key>APFSPhysicalStore</key>
			<string>disk0s2</string>
		</dict>
	</array>
	<key>RemovableMedia</key>
	<false/>
	<key>Size</key>
	<integer>994662584320</integer>
	<key>VolumeName</key>
	<string>Macintosh HD - Data</string>
	<key>VolumeUUID</key>
	<string>E908C7D5-563D-47CC-94E6-C7986C000433</string>
</dict>
</plist>
`

func TestParseDiskutilInfo(t *testing.T) {
	info, err := parseDiskutilInfo([]byte(diskutilFixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.VolumeUUID != "E908C7D5-563D-47CC-94E6-C7986C000433" {
		t.Fatalf("VolumeUUID = %q", info.VolumeUUID)
	}
	if info.VolumeName != "Macintosh HD - Data" {
		t.Fatalf("VolumeName = %q", info.VolumeName)
	}
	if !info.Internal || info.RemovableMedia || info.Ejectable {
		t.Fatalf("placement flags wrong: %+v", info)
	}
}

func TestParseDiskutilInfo_ExternalDriveClassifies(t *testing.T) {
	external := strings.ReplaceAll(diskutilFixture, "<key>Internal</key>\n\t<true/>", "<key>Internal</key>\n\t<false/>")
	external = strings.ReplaceAll(external, "<key>Ejectable</key>\n\t<false/>", "<key>Ejectable</key>\n\t<true/>")
	info, err := parseDiskutilInfo([]byte(external))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.Internal || !info.Ejectable {
		t.Fatalf("flags wrong: %+v", info)
	}
}

func TestParseDiskutilInfo_Malformed(t *testing.T) {
	for name, input := range map[string]string{
		"not xml":   "this is not a plist at all",
		"no dict":   `<?xml version="1.0"?><plist version="1.0"><string>lonely</string></plist>`,
		"truncated": diskutilFixture[:120],
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseDiskutilInfo([]byte(input)); err == nil {
				t.Fatal("malformed plist must error, got nil")
			}
		})
	}
}

// mountInfoFixture is a realistic /proc/self/mountinfo excerpt: root, a proc
// mount, an ext4 data disk, an escaped-space mount point, a cifs share, an nfs
// export, and one malformed line the parser must skip.
const mountInfoFixture = `21 1 259:2 / / rw,relatime shared:1 - ext4 /dev/nvme0n1p2 rw
22 21 0:5 / /proc rw,nosuid,nodev,noexec shared:12 - proc proc rw
96 21 8:17 / /mnt/photos rw,relatime shared:48 - ext4 /dev/sdb1 rw
97 21 8:33 / /mnt/my\040disk rw,relatime shared:50 - ext4 /dev/sdc1 rw
120 21 0:52 / /mnt/nas rw,relatime shared:60 - cifs //nas.local/photos rw,vers=3.1.1
121 21 0:53 / /mnt/archive rw,relatime shared:61 - nfs4 server.lan:/export/archive rw,addr=10.0.0.2
garbage line that fits no format
`

func TestParseMountInfo(t *testing.T) {
	entries, err := parseMountInfo([]byte(mountInfoFixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 6 {
		t.Fatalf("entries = %d, want 6 (malformed line skipped)", len(entries))
	}
	if entries[3].MountPoint != "/mnt/my disk" {
		t.Fatalf("escaped mount point = %q, want %q", entries[3].MountPoint, "/mnt/my disk")
	}
	if entries[4].FilesystemType != "cifs" || entries[4].Source != "//nas.local/photos" {
		t.Fatalf("cifs entry wrong: %+v", entries[4])
	}
}

func TestParseMountInfo_Malformed(t *testing.T) {
	if _, err := parseMountInfo([]byte("total garbage\nmore garbage\n")); err == nil {
		t.Fatal("all-malformed mountinfo must error, got nil")
	}
}

func TestBestMountFor(t *testing.T) {
	entries, err := parseMountInfo([]byte(mountInfoFixture))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path      string
		wantMount string
	}{
		{"/home/user/pictures/a.jpg", "/"},
		{"/mnt/photos/2024/shot.raw", "/mnt/photos"},
		{"/mnt/photos", "/mnt/photos"},
		{"/mnt/photosarchive/x", "/"}, // sibling prefix must NOT match /mnt/photos
		{"/mnt/nas/album/b.jpg", "/mnt/nas"},
	}
	for _, testCase := range cases {
		entry, ok := bestMountFor(entries, testCase.path)
		if !ok || entry.MountPoint != testCase.wantMount {
			t.Errorf("bestMountFor(%q) = %q (ok=%v), want %q", testCase.path, entry.MountPoint, ok, testCase.wantMount)
		}
	}
}

func TestParseSMBSource(t *testing.T) {
	share, err := parseSMBSource("//NAS.local/Photos")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if share.Identity != "smb://nas.local/photos" || share.Host != "NAS.local" || share.Share != "Photos" {
		t.Fatalf("share = %+v", share)
	}
	// The macOS mount_smbfs form carries the user — identity must drop it.
	withUser, err := parseSMBSource("//ari@nas.local/Photos")
	if err != nil {
		t.Fatalf("parse with user: %v", err)
	}
	if withUser.Identity != "smb://nas.local/photos" {
		t.Fatalf("user part must not enter the identity: %q", withUser.Identity)
	}
	// SMB shares are case-insensitive server-side and mount forms vary by hand-typed
	// case/slash — every variant must fold to ONE identity (no duplicate volume rows).
	for _, variant := range []string{"//nas.local/photos", "//NAS.LOCAL/PHOTOS/", "//ari@nas.local/Photos/"} {
		folded, foldErr := parseSMBSource(variant)
		if foldErr != nil {
			t.Fatalf("parse %q: %v", variant, foldErr)
		}
		if folded.Identity != "smb://nas.local/photos" {
			t.Errorf("parseSMBSource(%q).Identity = %q, want the folded form", variant, folded.Identity)
		}
	}
}

func TestParseSMBSource_Malformed(t *testing.T) {
	for _, input := range []string{"", "/dev/sda1", "//hostonly", "//"} {
		if _, err := parseSMBSource(input); err == nil {
			t.Errorf("parseSMBSource(%q) must error", input)
		}
	}
}

func TestParseNFSSource(t *testing.T) {
	share, err := parseNFSSource("Server.lan:/export/archive")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if share.Identity != "nfs://server.lan/export/archive" || share.Host != "Server.lan" || share.Share != "/export/archive" {
		t.Fatalf("share = %+v", share)
	}
	// Trailing slash folds; export case does NOT (unix paths are case-sensitive).
	trailing, err := parseNFSSource("Server.lan:/export/archive/")
	if err != nil {
		t.Fatalf("parse trailing: %v", err)
	}
	if trailing.Identity != "nfs://server.lan/export/archive" {
		t.Errorf("trailing slash must fold: %q", trailing.Identity)
	}
	upper, err := parseNFSSource("server.lan:/Export/Archive")
	if err != nil {
		t.Fatalf("parse upper: %v", err)
	}
	if upper.Identity != "nfs://server.lan/Export/Archive" {
		t.Errorf("export case must be preserved: %q", upper.Identity)
	}
}

func TestParseNFSSource_Malformed(t *testing.T) {
	for _, input := range []string{"", "/dev/sda1", "host:relative", "justhost"} {
		if _, err := parseNFSSource(input); err == nil {
			t.Errorf("parseNFSSource(%q) must error", input)
		}
	}
}

func TestUnescapeUdevName(t *testing.T) {
	if got := unescapeUdevName(`My\x20Disk`); got != "My Disk" {
		t.Fatalf("unescape = %q", got)
	}
	// Malformed escapes pass through untouched rather than corrupting the name.
	if got := unescapeUdevName(`Bad\xZZEscape\x2`); got != `Bad\xZZEscape\x2` {
		t.Fatalf("malformed escape mangled: %q", got)
	}
}
