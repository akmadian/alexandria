package seam

import (
	"github.com/charmbracelet/log"

	"github.com/akmadian/alexandria/internal/settings"
)

// settingsStore and keybindingsStore are the config slices the seam needs. Each is
// satisfied by a *settings.configFile[T] (Get is a cached read, Save writes
// atomically and updates the cache), so the service stays testable with a fake.
type settingsStore interface {
	Get() settings.Settings
	Save(settings.Settings) error
}

type keybindingsStore interface {
	Get() settings.Keybindings
	Save(settings.Keybindings) error
}

// SettingsService exposes the catalog-scoped settings and the keybinding override
// map (ledger #4/#5). The shapes are the internal/settings Go types verbatim — no
// parallel seam struct — so the generated TS tracks them (C13); the stale
// contract.ts Settings is reconciled against this in the deferred wails pass.
//
// Machine-scoped tuning (machine.json: worker pools, dependency paths) is
// deliberately NOT exposed here — it has no UI yet (DEFERRED §7). Keybinding
// CONFLICT detection is frontend-owned: the backend never interprets a commandID
// or chord (settings.go), so no keybinding_conflict code originates here.
type SettingsService struct {
	settings    settingsStore
	keybindings keybindingsStore
}

// NewSettingsService constructs the bound service over the settings + keybindings
// config files (the ones settings.Service opens alongside the catalog).
func NewSettingsService(settingsFile settingsStore, keybindingsFile keybindingsStore) *SettingsService {
	return &SettingsService{settings: settingsFile, keybindings: keybindingsFile}
}

// GetSettings returns the current catalog-scoped settings.
func (s *SettingsService) GetSettings() settings.Settings {
	return s.settings.Get()
}

// UpdateSettings replaces the settings with the given value (read-modify-write on
// the frontend, whole-object set here). One set absorbs field growth — a new
// setting is a new struct field, not a new method. Note: Save caches the value
// verbatim; the sanitizer (field clamping) runs only on a cold load / hot-reload,
// so an out-of-range field is served back unclamped until the next reload. The
// frontend is expected to send in-range values; the sanitizer is the backstop for
// hand-edited files, not for this path.
//
//nolint:gocritic // hugeParam: value signature required to match configFile[T].Save (T by value).
func (s *SettingsService) UpdateSettings(next settings.Settings) error {
	if err := s.settings.Save(next); err != nil {
		log.Error("seam: UpdateSettings failed", "err", err)
		return normalizeError(err)
	}
	log.Info("seam: updated settings")
	return nil
}

// ListKeybindings returns the user's keybinding override map (commandID → chord).
// It is overrides only — the frontend command registry owns the full default set.
func (s *SettingsService) ListKeybindings() settings.Keybindings {
	return s.keybindings.Get()
}

// SetKeybinding sets one override. An empty chord removes the override (falling the
// command back to its registry default), so the map never accumulates dead keys.
func (s *SettingsService) SetKeybinding(commandID, chord string) error {
	current := s.keybindings.Get()
	next := make(settings.Keybindings, len(current)+1)
	for command, existing := range current {
		next[command] = existing
	}
	if chord == "" {
		delete(next, commandID)
	} else {
		next[commandID] = chord
	}
	if err := s.keybindings.Save(next); err != nil {
		log.Error("seam: SetKeybinding failed", "command", commandID, "err", err)
		return normalizeError(err)
	}
	log.Info("seam: set keybinding", "command", commandID, "chord", chord)
	return nil
}

// ResetKeybindings clears every override, restoring the registry defaults.
func (s *SettingsService) ResetKeybindings() error {
	if err := s.keybindings.Save(settings.Keybindings{}); err != nil {
		log.Error("seam: ResetKeybindings failed", "err", err)
		return normalizeError(err)
	}
	log.Info("seam: reset keybindings")
	return nil
}
