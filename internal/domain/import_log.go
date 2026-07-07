package domain

import "time"

// ImportSession is one run of the pipeline (import / reconcile / watch). It is
// system-log data, not catalog truth — losable, and the durable home of the
// per-run counts plus the per-extension skip tallies. It is also the parent of
// the DLQ rows (ImportError) minted during the run.
type ImportSession struct {
	ID         string
	SourceID   string
	Kind       string // import | reconcile | watch
	StartedAt  time.Time
	FinishedAt *time.Time
	Added      int
	Updated    int
	Moved      int
	Skipped    int
	Dups       int
	Errors     int
	// SkippedUnknown tallies files skipped for an unrecognized extension, keyed
	// by extension; SkippedIgnored tallies ignore-list hits, keyed by pattern.
	SkippedUnknown map[string]int
	SkippedIgnored map[string]int
}

// ImportError is one DLQ row: a file that failed a stage. It never aborts the
// run — the pipeline records it and moves on. Re-drive is passive (D13): the
// next file event, any reconcile, or a manual retry re-feeds the path.
type ImportError struct {
	ID         string
	SessionID  string
	Path       string
	Stage      string // scan | hash | match | extract | thumb | write
	ReasonCode string // machine-readable taxonomy, e.g. "decode_failed"
	Message    string
	Attempts   int
	OccurredAt time.Time
}
