package seam

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/charmbracelet/log"

	"github.com/akmadian/alexandria/internal/ast"
	"github.com/akmadian/alexandria/internal/domain"
)

// This file is the seam's error-normalization layer (impl/15 §4). Every bound
// method funnels its engine error through normalizeError, so exactly one shape
// crosses the seam: an ApiError carrying a coarse kind and a stable code. Display
// text is frontend-owned (C14) — codes cross, strings do not. The generator
// (./generate) publishes the ApiErrorKind and ErrorCode consts below to
// frontend/src/_generated-types/errors.ts, so the frontend switches on generated
// literals, not hand-copied strings.

// ApiErrorKind is the coarse lane the UI branches on first. Mirrors contract.ts.
type ApiErrorKind string

const (
	// KindTransport is IPC/Wails failure. Never produced here — it surfaces on
	// the JS side when a call never reaches Go; declared so the union is complete.
	KindTransport ApiErrorKind = "transport"
	// KindDegraded is a missing dependency / nil capability (C10's one-fallback
	// lane): the engine is up but something it needs is absent.
	KindDegraded ApiErrorKind = "degraded"
	// KindDomain is an expected, named rule violation the UI renders as guidance
	// (not found, invalid query, conflict).
	KindDomain ApiErrorKind = "domain"
	// KindUnexpected is anything unrecognized: logged with its underlying message
	// here, and crosses the seam with no code and no raw string.
	KindUnexpected ApiErrorKind = "unexpected"
)

// ErrorCode is the fine-grained, stable domain code. Add a const here and the
// generator surfaces it in TS automatically — the consts are the single source of
// truth (same discovery mechanism as the domain enums). Only codes the Go side
// actually produces live here; frontend-owned rules (e.g. keybinding conflicts,
// which the backend never interprets) are not codes we emit.
type ErrorCode string

const (
	CodeNotFound           ErrorCode = "not_found"
	CodeQueryInvalid       ErrorCode = "query_invalid"
	CodeQueryVersionTooNew ErrorCode = "query_version_too_new"
	CodeConflict           ErrorCode = "conflict"
	CodeValidation         ErrorCode = "validation"
	CodeVolumeOffline      ErrorCode = "volume_offline"
)

// ApiError is the single wire error shape. It implements error, and its Error()
// is compact JSON so the kind + code survive Wails's error→string serialization:
// the frontend's transport adapter parses this back into its ApiError class. It is
// never unwrapped further — normalizeError has already classified it.
type ApiError struct {
	Kind   ApiErrorKind `json:"kind"`
	Code   ErrorCode    `json:"code,omitempty"`
	Detail string       `json:"detail,omitempty"`
}

func (e *ApiError) Error() string {
	encoded, err := json.Marshal(e)
	if err != nil {
		// Unreachable for these plain-string fields; fall back to the kind rather
		// than leak a raw engine string across the seam.
		return string(e.Kind)
	}
	return string(encoded)
}

// normalizeError maps an engine error to the wire ApiError. Known, expected error
// types get a typed code the UI renders; anything unrecognized is logged with its
// message and returned as KindUnexpected (no code, no raw string). A nil error
// returns nil, and an already-normalized ApiError passes through unchanged (so
// nested seam calls don't double-wrap).
func normalizeError(err error) error {
	if err == nil {
		return nil
	}

	var alreadyNormalized *ApiError
	if errors.As(err, &alreadyNormalized) {
		return alreadyNormalized
	}

	var (
		notFound      *domain.NotFoundError
		conflict      *domain.ConflictError
		validation    *domain.ValidationError
		volumeOffline *domain.VolumeOfflineError
		versionTooNew *ast.ErrVersionTooNew
	)
	switch {
	case errors.As(err, &notFound):
		return &ApiError{Kind: KindDomain, Code: CodeNotFound, Detail: notFound.Error()}
	case errors.As(err, &conflict):
		return &ApiError{Kind: KindDomain, Code: CodeConflict, Detail: conflict.Error()}
	case errors.As(err, &validation):
		return &ApiError{Kind: KindDomain, Code: CodeValidation, Detail: validation.Error()}
	case errors.As(err, &volumeOffline):
		return &ApiError{Kind: KindDomain, Code: CodeVolumeOffline, Detail: volumeOffline.Error()}
	case errors.As(err, &versionTooNew):
		return &ApiError{Kind: KindDomain, Code: CodeQueryVersionTooNew, Detail: versionTooNew.Error()}
	case isQueryValidationError(err):
		return &ApiError{Kind: KindDomain, Code: CodeQueryInvalid, Detail: err.Error()}
	default:
		log.Error("seam: unexpected error", "err", err)
		return &ApiError{Kind: KindUnexpected}
	}
}

// isQueryValidationError reports whether err is any of ast.Validate's structured
// grammar/value/structure errors — all of which map to one query_invalid code
// (the specific field/operator detail rides in Detail for logs, not as branching
// codes the UI must exhaustively handle).
func isQueryValidationError(err error) bool {
	var (
		unknownField    *ast.ErrUnknownField
		invalidOperator *ast.ErrInvalidOperator
		invalidValue    *ast.ErrInvalidValue
		structure       *ast.ErrStructure
	)
	return errors.As(err, &unknownField) ||
		errors.As(err, &invalidOperator) ||
		errors.As(err, &invalidValue) ||
		errors.As(err, &structure)
}

// seamContext is the context every bound method runs under. Wails v2 gives bound
// methods no per-call context, and these are short synchronous engine calls, so a
// background context is the correct scope today.
//
// ponytail: swap to the Wails startup context (app-lifetime, cancels on quit) if a
// long-running bound call ever lands — one change here, not one per method.
func seamContext() context.Context { return context.Background() }
