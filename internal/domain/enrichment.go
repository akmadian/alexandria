package domain

import "time"

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
