package domain

import "time"

// EnrichmentKind names one enrichment job kind — the stable vocabulary shared by
// the engine registry (the Kind key), the DLQ, and the seam decoration. It is the
// self-describing wire form of "what is enriching / failed" on an asset row: the
// seam ships kind names, never a registry-order-dependent bitmask (task 21). The
// generator publishes this as a TS union; a crosswalk test pins it to the engine
// registry's kinds so the two cannot drift.
type EnrichmentKind string

const (
	EnrichmentKindThumbnail EnrichmentKind = "thumbnail"
	EnrichmentKindSharpness EnrichmentKind = "sharpness"
	EnrichmentKindClipping  EnrichmentKind = "clipping"
	EnrichmentKindPhash     EnrichmentKind = "phash"
)

// EnrichmentError is one row of the enrichment DLQ (D28): a durable record that
// producing one artifact for one asset failed. Keyed (AssetID, Kind) — the
// natural key of post-identity work, unlike the path-keyed import DLQ. Its job
// is disambiguating absence: a missing artifact plus a row here reads "failed",
// not "pending". Attempts accumulates across scans; the missing-artifact scan
// stops re-enqueuing once it exhausts the attempt budget.
type EnrichmentError struct {
	AssetID       string
	Kind          string
	ReasonCode    string
	Message       string
	Attempts      int
	LastAttemptAt time.Time
}
