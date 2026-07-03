package domain

import "fmt"

type NotFoundError struct {
	Resource string
	ID       string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s not found: %s", e.Resource, e.ID)
}

type ConflictError struct {
	Resource string
	Field    string
	Message  string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("conflict on %s.%s: %s", e.Resource, e.Field, e.Message)
}

type SourceOfflineError struct {
	SourceID string
	Path     string
}

func (e *SourceOfflineError) Error() string {
	return fmt.Sprintf("source %s is offline (path: %s)", e.SourceID, e.Path)
}

type CatalogLockedError struct {
	Path string
}

func (e *CatalogLockedError) Error() string {
	return fmt.Sprintf("catalog is locked: %s", e.Path)
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error on %s: %s", e.Field, e.Message)
}

type ErrKeybindingConflict struct {
	Combo          string
	ConflictAction string
}

func (e *ErrKeybindingConflict) Error() string {
	return fmt.Sprintf("keybinding %s conflicts with action %s", e.Combo, e.ConflictAction)
}

type ErrSchemaTooOld struct {
	Current  int
	Required int
}

func (e *ErrSchemaTooOld) Error() string {
	return fmt.Sprintf("schema version %d is too old, requires %d", e.Current, e.Required)
}

type ErrSchemaTooNew struct {
	Current int
	Known   int
}

func (e *ErrSchemaTooNew) Error() string {
	return fmt.Sprintf("schema version %d is newer than known version %d — please update the app", e.Current, e.Known)
}
