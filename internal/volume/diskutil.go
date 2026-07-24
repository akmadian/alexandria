package volume

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
)

// This file is the PURE half of the macOS identity probe (§1 discipline): bytes
// of `diskutil info -plist <mountpoint>` in, facts out. The subprocess call that
// produces the bytes is thin orchestration in prober_darwin.go. Tagless so the
// parser unit-tests run on every platform.

// diskutilInfo is the slice of diskutil's volume record the prober consumes.
type diskutilInfo struct {
	VolumeUUID     string
	VolumeName     string
	Internal       bool
	RemovableMedia bool
	Ejectable      bool
}

// parseDiskutilInfo decodes the top-level dict of an Apple XML plist as emitted
// by `diskutil info -plist`, extracting the keys the prober needs. Nested
// containers (arrays, sub-dicts) are skipped wholesale — every key of interest
// is a top-level scalar. Unknown keys are ignored; malformed XML is an error.
func parseDiskutilInfo(plistBytes []byte) (diskutilInfo, error) {
	decoder := xml.NewDecoder(bytes.NewReader(plistBytes))

	var info diskutilInfo
	var inTopDict bool
	var pendingKey string
	sawDict := false

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return diskutilInfo{}, fmt.Errorf("diskutil plist: %w", err)
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "dict":
				if !inTopDict {
					inTopDict = true
					sawDict = true
					continue
				}
				// A nested dict VALUE inside the top dict: skip it wholesale —
				// every key of interest is a top-level scalar.
				if err := decoder.Skip(); err != nil {
					return diskutilInfo{}, fmt.Errorf("diskutil plist: %w", err)
				}
				pendingKey = ""
			case "array":
				if err := decoder.Skip(); err != nil {
					return diskutilInfo{}, fmt.Errorf("diskutil plist: %w", err)
				}
				pendingKey = ""
			case "key":
				if inTopDict {
					var name string
					if err := decoder.DecodeElement(&name, &element); err != nil {
						return diskutilInfo{}, fmt.Errorf("diskutil plist: %w", err)
					}
					pendingKey = name
				}
			case "string", "integer":
				if inTopDict && pendingKey != "" {
					var value string
					if err := decoder.DecodeElement(&value, &element); err != nil {
						return diskutilInfo{}, fmt.Errorf("diskutil plist: %w", err)
					}
					applyDiskutilString(&info, pendingKey, value)
					pendingKey = ""
				} else if err := decoder.Skip(); err != nil {
					return diskutilInfo{}, fmt.Errorf("diskutil plist: %w", err)
				}
			case "true", "false":
				if inTopDict && pendingKey != "" {
					applyDiskutilBool(&info, pendingKey, element.Name.Local == "true")
					pendingKey = ""
				}
				if err := decoder.Skip(); err != nil {
					return diskutilInfo{}, fmt.Errorf("diskutil plist: %w", err)
				}
			}
		case xml.EndElement:
			if element.Name.Local == "dict" {
				inTopDict = false
			}
		}
	}
	if !sawDict {
		return diskutilInfo{}, fmt.Errorf("diskutil plist: no dict found (not a plist?)")
	}
	return info, nil
}

func applyDiskutilString(info *diskutilInfo, key, value string) {
	switch key {
	case "VolumeUUID":
		info.VolumeUUID = value
	case "VolumeName":
		info.VolumeName = value
	}
}

func applyDiskutilBool(info *diskutilInfo, key string, value bool) {
	switch key {
	case "Internal":
		info.Internal = value
	case "RemovableMedia":
		info.RemovableMedia = value
	case "Ejectable":
		info.Ejectable = value
	}
}
