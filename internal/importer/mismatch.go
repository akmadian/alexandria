package importer

import (
	"fmt"

	"github.com/akmadian/alexandria/internal/assettype"
)

// preciseRasterFamily maps the stdlib-decodable raster extensions to the exact
// content family their bytes MUST show. These are the only formats where an
// extension corresponds 1:1 to a magic signature, so a disagreement is
// unambiguous ("a .png that is really a JPEG"). Container/RAW families
// (TIFF shared by CR2/NEF/DNG, ISOBMFF shared by MOV/MP4/HEIC/M4A) are
// deliberately NOT validated here — one magic covers many extensions, so the
// extension legitimately refines the family (D7: "content confirms container,
// extension picks dialect").
var preciseRasterFamily = map[string]assettype.ContentFamily{
	"jpg":  assettype.FamilyJPEG,
	"jpeg": assettype.FamilyJPEG,
	"png":  assettype.FamilyPNG,
	"gif":  assettype.FamilyGIF,
	"bmp":  assettype.FamilyBMP,
	"webp": assettype.FamilyWebP,
}

// familyCanonicalExtension maps a sniffed raster family back to the canonical
// extension whose handler (MIME + capabilities) we adopt when reclassifying. We
// keep the CONTENT's handler/MIME (so a mislabeled JPEG still extracts + thumbs
// correctly) while leaving the asset's on-disk extension untouched.
var familyCanonicalExtension = map[assettype.ContentFamily]string{
	assettype.FamilyJPEG: "jpeg",
	assettype.FamilyPNG:  "png",
	assettype.FamilyGIF:  "gif",
	assettype.FamilyBMP:  "bmp",
	assettype.FamilyWebP: "webp",
}

// applyMismatchPolicy implements D7 for the unambiguous raster case. It runs on
// the head bytes the hash stage already read (zero extra I/O):
//
//   - agree, or Sniff can't identify the bytes → proceed on the extension.
//   - a precise raster extension whose bytes are a DIFFERENT known raster family
//     → trust the content: adopt that family's handler/MIME/type, drop an
//     extension_mismatch marker into extended_metadata, and log an informational
//     import_errors row. The asset still indexes.
//
// The third D7 branch (a supported container that sniffs as neither its claimed
// container nor anything usable → hard reject, no identity) is unreachable with
// the current Sniff table: an ok=false result means "unrecognized", which we
// trust the extension for, and any ok=true result is itself a usable family. If
// Sniff ever gains an explicit "corrupt/garbage" signal, wire the reject here.
func applyMismatchPolicy(item *pipelineItem) {
	expectedFamily, isPreciseRaster := preciseRasterFamily[item.scanned.ext]
	if !isPreciseRaster {
		return
	}
	detectedFamily, recognized := assettype.Sniff(item.head)
	if !recognized || detectedFamily == expectedFamily {
		return // unrecognized (trust ext) or agreement
	}
	canonicalExtension, isRaster := familyCanonicalExtension[detectedFamily]
	if !isRaster {
		return // sniffed something non-raster for a raster ext; too odd to auto-reclassify
	}
	handler, found := assettype.Classify(canonicalExtension)
	if !found {
		return
	}
	declaredExtension := item.scanned.ext
	item.scanned.mime = handler.MIME
	item.scanned.fileType = handler.Type
	item.scanned.handler = handler // EXTRACT/THUMB now dispatch off the content's handler
	item.mismatchMarker = map[string]any{
		"alexandria:extension_mismatch": map[string]any{
			"declared": declaredExtension,
			"detected": string(detectedFamily),
		},
	}
	item.addError("hash", "ext_mismatch",
		fmt.Sprintf("extension .%s but content is %s; trusting content", declaredExtension, detectedFamily))
}
