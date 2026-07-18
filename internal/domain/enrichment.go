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

// EnrichmentFailureCount is the DLQ rolled up by (kind, reason) for the debug
// snapshot (D28 commitment #4, task 22): Count rows total, Exhausted of them
// attempt-exhausted (terminally failed — the state the grid renders). It lives
// here, not in enrichment or sqlite, because the repo produces it and the engine
// serves it and neither may import the other (sqlite has no enrichment import;
// enrichment imports sqlite). Carries json tags — it is part of the snapshot
// wire contract.
type EnrichmentFailureCount struct {
	Kind      string `json:"kind"`
	Reason    string `json:"reason"`
	Count     int    `json:"count"`
	Exhausted int    `json:"exhausted"`
}
