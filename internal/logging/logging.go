// Package logging owns the app's one logger format, so every entrypoint (the
// Wails app host, the dev harness) produces identically-shaped logs. It is a
// LEAF: format only — callers decide the destination writer (stderr, a file, or
// both) and pass it in.
package logging

import (
	"io"
	"time"

	"github.com/charmbracelet/log"
)

// New returns the standard app logger writing to w: RFC3339 timestamps (a file
// log needs the date, not just the time), caller location, and Debug level so
// per-item detail is captured (coding-guidelines §4). Compose the writer at the
// call site — the app host tees stderr with a file via io.MultiWriter.
func New(w io.Writer) *log.Logger {
	logger := log.NewWithOptions(w, log.Options{
		ReportTimestamp: true,
		ReportCaller:    true,
		TimeFormat:      time.RFC3339,
	})
	logger.SetLevel(log.DebugLevel)
	return logger
}
