package seam_test

import (
	"errors"
	"testing"

	"github.com/akmadian/alexandria/internal/seam"
	"github.com/akmadian/alexandria/internal/settings"
)

// fakeSettings / fakeKeybindings are in-memory config stores holding the last
// saved value, matching the settings.configFile Get/Save surface.
type fakeSettings struct {
	value settings.Settings
	saved *settings.Settings
	err   error
}

func (f *fakeSettings) Get() settings.Settings { return f.value }

//nolint:gocritic // hugeParam: value signature required to satisfy the settingsStore.Save interface (mirrors configFile[T].Save).
func (f *fakeSettings) Save(next settings.Settings) error {
	if f.err != nil {
		return f.err
	}
	f.value = next
	f.saved = &next
	return nil
}

type fakeKeybindings struct {
	value settings.Keybindings
	err   error
}

func (f *fakeKeybindings) Get() settings.Keybindings {
	if f.value == nil {
		return settings.Keybindings{}
	}
	return f.value
}

func (f *fakeKeybindings) Save(next settings.Keybindings) error {
	if f.err != nil {
		return f.err
	}
	f.value = next
	return nil
}

func TestGetSettings_ReturnsStored(t *testing.T) {
	store := &fakeSettings{value: settings.Settings{ThumbnailQuality: 77}}
	service := seam.NewSettingsService(store, &fakeKeybindings{})

	if got := service.GetSettings(); got.ThumbnailQuality != 77 {
		t.Fatalf("got %+v", got)
	}
}

func TestUpdateSettings_Saves(t *testing.T) {
	store := &fakeSettings{}
	service := seam.NewSettingsService(store, &fakeKeybindings{})

	if err := service.UpdateSettings(settings.Settings{ThumbnailQuality: 55}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if store.saved == nil || store.saved.ThumbnailQuality != 55 {
		t.Fatalf("expected save of quality 55, got %+v", store.saved)
	}
}

func TestSetKeybinding_AddsThenEmptyChordRemoves(t *testing.T) {
	binds := &fakeKeybindings{value: settings.Keybindings{"rate_5": "5"}}
	service := seam.NewSettingsService(&fakeSettings{}, binds)

	if err := service.SetKeybinding("flag_pick", "p"); err != nil {
		t.Fatalf("SetKeybinding: %v", err)
	}
	if binds.value["flag_pick"] != "p" || binds.value["rate_5"] != "5" {
		t.Fatalf("expected both bindings, got %v", binds.value)
	}
	if err := service.SetKeybinding("rate_5", ""); err != nil {
		t.Fatalf("SetKeybinding remove: %v", err)
	}
	if _, present := binds.value["rate_5"]; present {
		t.Fatalf("empty chord should remove the override, got %v", binds.value)
	}
}

func TestResetKeybindings_ClearsAll(t *testing.T) {
	binds := &fakeKeybindings{value: settings.Keybindings{"rate_5": "5"}}
	service := seam.NewSettingsService(&fakeSettings{}, binds)

	if err := service.ResetKeybindings(); err != nil {
		t.Fatalf("ResetKeybindings: %v", err)
	}
	if len(binds.value) != 0 {
		t.Fatalf("expected empty overrides, got %v", binds.value)
	}
}

func TestListKeybindings_ReturnsStored(t *testing.T) {
	binds := &fakeKeybindings{value: settings.Keybindings{"rate_5": "5"}}
	service := seam.NewSettingsService(&fakeSettings{}, binds)

	if got := service.ListKeybindings(); got["rate_5"] != "5" {
		t.Fatalf("got %v", got)
	}
}

// Save failures must surface as a normalized ApiError on every write path, never a
// raw config error — one assertion per write method.
func TestSettingsWrites_SaveErrorMapsToUnexpected(t *testing.T) {
	t.Run("UpdateSettings", func(t *testing.T) {
		service := seam.NewSettingsService(&fakeSettings{err: errors.New("disk full")}, &fakeKeybindings{})
		assertUnexpected(t, service.UpdateSettings(settings.Settings{}))
	})
	t.Run("SetKeybinding", func(t *testing.T) {
		service := seam.NewSettingsService(&fakeSettings{}, &fakeKeybindings{err: errors.New("disk full")})
		assertUnexpected(t, service.SetKeybinding("flag_pick", "p"))
	})
	t.Run("ResetKeybindings", func(t *testing.T) {
		service := seam.NewSettingsService(&fakeSettings{}, &fakeKeybindings{err: errors.New("disk full")})
		assertUnexpected(t, service.ResetKeybindings())
	})
}
