package seam

import (
	"context"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// This file is the ONLY place runtime.EventsEmit is called (enforced by forbidigo
// in .golangci.yml). Everything else about events — the catalog, validation,
// envelope construction, the payload structs — is pure Go in events.go, so the
// services and their tests never depend on Wails. The engine's runtime-agnostic
// contract (D1) holds one level down: the engine hands the seam its existing
// callbacks (importer OnProgress, watcher connectivity), and the seam adapts them
// to this emitter; the engine itself imports no Wails.
//
// The Wails runtime package is webkit-free and cgo-free (verified: it builds and
// vets under the backend toolchain with no build tags), so importing it here does
// not pull the webview into the seam's checks.

// WailsEmitter pushes validated envelopes to the frontend over the Wails runtime.
// Its context arrives at OnStartup (after the services are constructed), so it is
// bound late: before Bind, Emit is a no-op — there is no webview to receive an
// event yet anyway. Bind may race with a background job's first Emit, so the
// context is mutex-guarded.
type WailsEmitter struct {
	mu  sync.RWMutex
	ctx context.Context
}

// NewWailsEmitter returns an unbound emitter. The composition root constructs it,
// hands it to the services via WithEmitter, and calls Bind in OnStartup.
func NewWailsEmitter() *WailsEmitter { return &WailsEmitter{} }

// Bind captures the Wails app context. Called once from OnStartup, before which
// Emit drops (window not up).
func (e *WailsEmitter) Bind(ctx context.Context) {
	e.mu.Lock()
	e.ctx = ctx
	e.mu.Unlock()
}

// Emit validates the event against the catalog and, if a context is bound, pushes
// the envelope on the topic's channel. Invalid events are logged and dropped by
// buildEnvelope; a pre-Bind emit is dropped silently. Fire-and-forget: Wails gives
// no delivery guarantee and events are hints (C8), so dropping is acceptable.
func (e *WailsEmitter) Emit(eventType EventType, payload any) {
	envelope, ok := buildEnvelope(eventType, payload)
	if !ok {
		return
	}
	e.mu.RLock()
	ctx := e.ctx
	e.mu.RUnlock()
	if ctx == nil {
		return
	}
	runtime.EventsEmit(ctx, string(envelope.Topic), envelope)
}
