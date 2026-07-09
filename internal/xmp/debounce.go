package xmp

import (
	"context"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

const defaultOutboundDelay = 2 * time.Second

// WriteBackDebouncer coalesces judgment changes into per-asset outbound writes.
// A 50-asset triage session produces 50 writes total, not per-keystroke. The
// composition root (app-host) owns the lifecycle; it creates the debouncer at
// startup and calls Close at shutdown.
type WriteBackDebouncer struct {
	syncer *Syncer
	delay  time.Duration
	logger *log.Logger

	mu     sync.Mutex
	timers map[string]*time.Timer // asset ID → pending write timer
	closed bool
}

func NewWriteBackDebouncer(syncer *Syncer, logger *log.Logger) *WriteBackDebouncer {
	if logger == nil {
		logger = log.Default()
	}
	return &WriteBackDebouncer{
		syncer: syncer,
		delay:  defaultOutboundDelay,
		logger: logger,
		timers: make(map[string]*time.Timer),
	}
}

// Schedule queues an outbound write for the given asset. If a write is already
// pending, the timer resets — the quiet period starts over. sidecarPath is the
// absolute path where the sidecar should be written.
func (d *WriteBackDebouncer) Schedule(assetID string, sidecarPath string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}

	if timer, ok := d.timers[assetID]; ok {
		timer.Stop()
	}

	d.timers[assetID] = time.AfterFunc(d.delay, func() {
		d.mu.Lock()
		delete(d.timers, assetID)
		d.mu.Unlock()

		d.fire(assetID, sidecarPath)
	})
	d.logger.Debug("xmp: outbound write scheduled", "asset", assetID, "delay", d.delay)
}

func (d *WriteBackDebouncer) fire(assetID string, sidecarPath string) {
	ctx := context.Background()
	asset, err := d.syncer.reader.Get(ctx, assetID)
	if err != nil {
		d.logger.Error("xmp: outbound debounce: load asset failed", "asset", assetID, "err", err)
		return
	}
	if err := d.syncer.writeOutbound(ctx, asset, sidecarPath); err != nil {
		d.logger.Error("xmp: outbound debounce: write failed", "asset", assetID, "err", err)
	}
}

func (d *WriteBackDebouncer) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	for id, timer := range d.timers {
		timer.Stop()
		delete(d.timers, id)
	}
}
