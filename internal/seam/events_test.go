package seam_test

import (
	"context"
	"sync"
	"testing"

	"github.com/akmadian/alexandria/internal/seam"
)

// capturedEvent is one emit recorded by fakeEmitter.
type capturedEvent struct {
	Type    seam.EventType
	Payload any
}

// fakeEmitter records every Emit for assertions, standing in for the WailsEmitter
// in service tests. Safe for the concurrent emits an import job produces.
type fakeEmitter struct {
	mu     sync.Mutex
	events []capturedEvent
}

func (f *fakeEmitter) Emit(eventType seam.EventType, payload any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, capturedEvent{Type: eventType, Payload: payload})
}

func (f *fakeEmitter) snapshot() []capturedEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]capturedEvent(nil), f.events...)
}

func (f *fakeEmitter) typesOf() []seam.EventType {
	events := f.snapshot()
	types := make([]seam.EventType, len(events))
	for i, event := range events {
		types[i] = event.Type
	}
	return types
}

// TestEventCatalog_IsInternallyConsistent is the C8 completeness gate: every
// declared event type maps to a known topic and a distinct, non-nil payload. It
// mirrors the registry MustValidate idiom (C10) — incompleteness fails the suite,
// not a user session.
func TestEventCatalog_IsInternallyConsistent(t *testing.T) {
	if err := seam.ValidateEventCatalog(); err != nil {
		t.Fatalf("event catalog invalid: %v", err)
	}
}

// TestNopEmitter_DropsSilently confirms the default emitter is a no-op (a service
// built without WithEmitter must still function; events are hints).
func TestNopEmitter_DropsSilently(t *testing.T) {
	// A service constructed with no emitter option must not panic on a write path.
	// Exercised indirectly here by constructing one and relying on the write tests;
	// this test documents the contract explicitly.
	service := seam.NewAssetService(&fakeAssets{}, &fakeAssets{})
	if service == nil {
		t.Fatal("service should construct without an emitter")
	}
}

// TestWailsEmitter_SafeWithoutWebview covers the emitter's guard logic without a
// live webview: an emit before Bind is dropped (no context), and an invalid event
// is dropped by validation before any runtime call. Neither path reaches
// runtime.EventsEmit, which is exercised only at wails-dev time.
func TestWailsEmitter_SafeWithoutWebview(t *testing.T) {
	emitter := seam.NewWailsEmitter()
	// Pre-Bind: a well-formed event has nowhere to go and is dropped, no panic.
	emitter.Emit(seam.EventCatalogChanged, seam.CatalogChange{Scope: seam.ScopeAssets})
	// After Bind, an uncataloged event is rejected by validation before the runtime
	// call — so this stays safe without a real Wails app in the context.
	emitter.Bind(context.Background())
	emitter.Emit(seam.EventType("bogus"), seam.CatalogChange{})
}
