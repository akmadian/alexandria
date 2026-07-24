package xmp

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/ast"
	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/charmbracelet/log"
)

// countingReader records reader.Get calls and always errors, so the debouncer's
// fire() reaches Get (proving a timer fired) but stops before writeOutbound — the
// coalescing behavior is observable with no exiftool daemon in play.
type countingReader struct{ gets atomic.Int32 }

func (r *countingReader) Get(context.Context, string) (*domain.Asset, error) {
	r.gets.Add(1)
	return nil, errors.New("stub: no asset")
}
func (r *countingReader) FindByHash(context.Context, string, int64) (*domain.Asset, error) {
	panic("unused")
}
func (r *countingReader) FindByVolumePath(context.Context, string, string) (*domain.Asset, error) {
	panic("unused")
}
func (r *countingReader) ListKnownFiles(context.Context, string, string) (map[string]domain.FileStat, error) {
	panic("unused")
}
func (r *countingReader) ListPathsStatus(context.Context, string, string) ([]catalog.PathStatus, error) {
	panic("unused")
}
func (r *countingReader) QueryAssets(context.Context, ast.Query, ast.Arrangement, ast.Page) ([]catalog.AssetRow, int, error) {
	panic("unused")
}
func (r *countingReader) AssetIDSlice(context.Context, ast.Query, ast.Arrangement, int, int) ([]string, error) {
	panic("unused")
}
func (r *countingReader) IndexOfAsset(context.Context, ast.Query, ast.Arrangement, string) (*int, error) {
	panic("unused")
}
func (r *countingReader) DistinctValues(context.Context, ast.Field) ([]string, error) {
	panic("unused")
}
func (r *countingReader) ReadTriageStates(context.Context, []string) ([]catalog.TriageState, error) {
	panic("unused")
}

// fastDebouncer builds a debouncer with a short delay (the real 2s default would
// make these tests crawl) wired to a stub reader.
func fastDebouncer(reader catalog.AssetReader) *WriteBackDebouncer {
	return &WriteBackDebouncer{
		syncer: &Syncer{reader: reader, logger: log.New(io.Discard)},
		delay:  20 * time.Millisecond,
		logger: log.New(io.Discard),
		timers: map[string]*time.Timer{},
	}
}

// A triage storm on ONE asset (repeated Schedule inside the quiet window) collapses
// to a single write — the debouncer's whole reason to exist (50 edits, not 50 writes).
func TestDebouncer_CoalescesRepeatedSchedules(t *testing.T) {
	reader := &countingReader{}
	d := fastDebouncer(reader)
	defer d.Close()

	for i := 0; i < 5; i++ {
		d.Schedule("asset-1", "/tmp/a.xmp")
		time.Sleep(3 * time.Millisecond) // resets the 20ms timer each time
	}
	time.Sleep(120 * time.Millisecond)

	if got := reader.gets.Load(); got != 1 {
		t.Fatalf("repeated schedules should coalesce to 1 write, got %d", got)
	}
}

// Distinct assets keep independent timers: three assets → three writes.
func TestDebouncer_PerAssetTimers(t *testing.T) {
	reader := &countingReader{}
	d := fastDebouncer(reader)
	defer d.Close()

	d.Schedule("asset-a", "/tmp/a.xmp")
	d.Schedule("asset-b", "/tmp/b.xmp")
	d.Schedule("asset-c", "/tmp/c.xmp")
	time.Sleep(120 * time.Millisecond)

	if got := reader.gets.Load(); got != 3 {
		t.Fatalf("three distinct assets should fire 3 writes, got %d", got)
	}
}

// Close cancels pending timers: a write scheduled but not yet fired never runs.
func TestDebouncer_CloseCancelsPending(t *testing.T) {
	reader := &countingReader{}
	d := fastDebouncer(reader)

	d.Schedule("asset-1", "/tmp/a.xmp")
	d.Close() // before the 20ms timer fires
	time.Sleep(120 * time.Millisecond)

	if got := reader.gets.Load(); got != 0 {
		t.Fatalf("Close must cancel the pending write, but %d fired", got)
	}
}

// Scheduling after Close is a no-op, not a panic or a leaked timer.
func TestDebouncer_ScheduleAfterCloseIsNoop(t *testing.T) {
	reader := &countingReader{}
	d := fastDebouncer(reader)

	d.Close()
	d.Schedule("asset-1", "/tmp/a.xmp")
	time.Sleep(120 * time.Millisecond)

	if got := reader.gets.Load(); got != 0 {
		t.Fatalf("Schedule after Close must not fire, but %d did", got)
	}
}
