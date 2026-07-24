package volume

import (
	"fmt"
	"strings"
)

// This file is the PURE half of the Linux identity probe plus the network-mount
// source parsers both unix probers share (§1 discipline): text in, facts out.
// The file reads and subprocess calls are thin orchestration in the per-OS
// prober files. Tagless so the parser unit-tests run on every platform.

// mountInfoEntry is one line of /proc/self/mountinfo, reduced to the fields the
// prober consumes.
type mountInfoEntry struct {
	MountPoint     string // where the filesystem is mounted (unescaped)
	FilesystemType string // e.g. ext4, btrfs, cifs, nfs4, tmpfs
	Source         string // the mount source: /dev/sda1, //host/share, host:/export, …
}

// parseMountInfo parses /proc/self/mountinfo content. Line format (proc(5)):
//
//	36 35 98:0 /mnt1 /mnt2 rw,noatime master:1 - ext3 /dev/root rw,errors=continue
//	^mountID ^parent ^maj:min ^root ^mountpoint ^options ^optional… ^- ^fstype ^source ^superopts
//
// The optional-fields run is variable-length and ends at the "-" separator.
// Malformed lines are skipped (a bad line must not blind the prober to the
// others); a file with no parseable line at all is an error.
func parseMountInfo(content []byte) ([]mountInfoEntry, error) {
	var entries []mountInfoEntry
	for _, line := range strings.Split(string(content), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		entry, ok := parseMountInfoLine(line)
		if ok {
			entries = append(entries, entry)
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("mountinfo: no parseable entries")
	}
	return entries, nil
}

func parseMountInfoLine(line string) (mountInfoEntry, bool) {
	fields := strings.Fields(line)
	// Minimum: 5 pre-separator fields + "-" + fstype + source.
	if len(fields) < 8 {
		return mountInfoEntry{}, false
	}
	separator := -1
	for index := 6; index < len(fields); index++ { // optional fields start at index 6
		if fields[index] == "-" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+2 >= len(fields) {
		return mountInfoEntry{}, false
	}
	return mountInfoEntry{
		MountPoint:     unescapeMountField(fields[4]),
		FilesystemType: fields[separator+1],
		Source:         unescapeMountField(fields[separator+2]),
	}, true
}

// unescapeMountField decodes the octal escapes proc uses for whitespace in
// mount fields: \040 space, \011 tab, \012 newline, \134 backslash.
func unescapeMountField(field string) string {
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return replacer.Replace(field)
}

// bestMountFor picks the entry whose mount point is the longest path-prefix of
// absolutePath — the filesystem the path actually lives on. ok=false when no
// entry contains the path (should not happen: "/" is always an entry).
func bestMountFor(entries []mountInfoEntry, absolutePath string) (mountInfoEntry, bool) {
	var best mountInfoEntry
	found := false
	for _, entry := range entries {
		if !pathHasPrefix(absolutePath, entry.MountPoint) {
			continue
		}
		if !found || len(entry.MountPoint) > len(best.MountPoint) {
			best = entry
			found = true
		}
	}
	return best, found
}

// pathHasPrefix reports whether path is at or under root in slash-path space.
func pathHasPrefix(path, root string) bool {
	if root == "/" {
		return strings.HasPrefix(path, "/")
	}
	return path == root || strings.HasPrefix(path, root+"/")
}

// networkShare is a parsed SMB/NFS mount source: the D24 identity for network
// mounts (host+share — they have no filesystem UUID).
type networkShare struct {
	Identity string // deterministic key: "smb://host/share" or "nfs://host/export"
	Host     string
	Share    string
}

// parseSMBSource parses an SMB/CIFS mount source of the form //host/share
// (optionally //user@host/share, the macOS mount_smbfs form; the user part is
// dropped — identity is host+share, not who mounted it).
func parseSMBSource(source string) (networkShare, error) {
	trimmed := strings.TrimPrefix(source, "//")
	if trimmed == source {
		return networkShare{}, fmt.Errorf("smb source %q: missing // prefix", source)
	}
	if at := strings.LastIndex(trimmed, "@"); at >= 0 {
		trimmed = trimmed[at+1:]
	}
	host, share, ok := strings.Cut(trimmed, "/")
	share = strings.TrimSuffix(share, "/")
	if !ok || host == "" || share == "" {
		return networkShare{}, fmt.Errorf("smb source %q: want //host/share", source)
	}
	// SMB share names are case-insensitive server-side, so the identity folds
	// case (and the trailing slash) — otherwise //nas/Photos remounted as
	// //nas/photos/ would mint a second volume row for the same share.
	return networkShare{
		Identity: "smb://" + strings.ToLower(host) + "/" + strings.ToLower(share),
		Host:     host,
		Share:    share,
	}, nil
}

// parseNFSSource parses an NFS mount source of the form host:/export.
func parseNFSSource(source string) (networkShare, error) {
	host, export, ok := strings.Cut(source, ":")
	if export != "/" {
		export = strings.TrimSuffix(export, "/")
	}
	if !ok || host == "" || export == "" || !strings.HasPrefix(export, "/") {
		return networkShare{}, fmt.Errorf("nfs source %q: want host:/export", source)
	}
	// Export paths stay case-sensitive (unix paths); only the trailing slash
	// folds, so host:/export/ and host:/export share one identity.
	return networkShare{
		Identity: "nfs://" + strings.ToLower(host) + export,
		Host:     host,
		Share:    export,
	}, nil
}

// unescapeUdevName decodes udev's \xNN hex escapes in /dev/disk/by-label names
// (a label "My Disk" publishes as "My\x20Disk"). Pure; the directory scan that
// feeds it lives in the linux prober.
func unescapeUdevName(name string) string {
	var out strings.Builder
	out.Grow(len(name))
	for index := 0; index < len(name); {
		if name[index] == '\\' && index+3 < len(name) && name[index+1] == 'x' {
			if high, low := hexValue(name[index+2]), hexValue(name[index+3]); high >= 0 && low >= 0 {
				out.WriteByte(byte(high<<4 | low)) //nolint:gosec // both nibbles are 0–15; the value fits a byte by construction
				index += 4
				continue
			}
		}
		out.WriteByte(name[index])
		index++
	}
	return out.String()
}

func hexValue(character byte) int {
	switch {
	case character >= '0' && character <= '9':
		return int(character - '0')
	case character >= 'a' && character <= 'f':
		return int(character-'a') + 10
	case character >= 'A' && character <= 'F':
		return int(character-'A') + 10
	default:
		return -1
	}
}
