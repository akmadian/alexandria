# All checks live here — Go only operates at the module root, so there is no
# pretending with per-directory Makefiles. Same checks CI runs: use
# `make check` (or `make check-backend`) before pushing.
.PHONY: check check-backend check-frontend check-app tidy-check-backend build-backend lint-backend vulncheck-backend test-backend cover-backend generate generate-seam check-generated

# Ratchet up as areas gain tests; never down. Excludes cmd/dev + testutil (wiring
# and test support by design). Measured 74.0% at gate creation (2026-07-09).
COVERAGE_MIN := 70

# The engine packages — everything the backend checks touch. Deliberately
# excludes the root Wails app package (main.go/app.go/logging.go), which needs the
# gtk/webkit toolchain to build; it has its own `make check-app` (impl/14). This
# is what lets backend work stay fast and toolchain-free (impl/14 §2, decision 4).
BACKEND_PKGS := ./internal/... ./cmd/...

# The root Wails package needs the platform webkit toolchain to build. Linux (CI,
# ubuntu 24.04) ships webkit2gtk-4.1 only → -tags webkit2_41; macOS uses native
# webkit, no tag. Detected so `make check-app` works on both (impl/14 §2, dec. 4).
WEBKIT_TAGS := $(if $(filter Linux,$(shell uname -s)),-tags webkit2_41,)

check:
	@failed=0; \
	$(MAKE) --no-print-directory check-backend || failed=1; \
	$(MAKE) --no-print-directory check-frontend || failed=1; \
	if [ $$failed -eq 0 ]; then \
		printf '\n\033[1;42;37m  ✓ ALL CHECKS PASSED  \033[0m\n\n'; \
	else \
		printf '\n\033[1;41;37m  ✗ CHECKS FAILED  \033[0m\n\n'; \
		exit 1; \
	fi

# --- code generation ---

# Regenerate the frontend types (vocabulary.ts from internal/ast, enums.ts from
# internal/domain). Run after changing the Go vocabulary or a domain enum; output
# is committed and check-generated gates it. Webkit-free — pure Go, prints TS.
generate:
	go run ./cmd/generate -out frontend/src/_generated-types -docs docs

# Back-compat alias — the generator moved from internal/seam/generate to
# cmd/generate (C15: it projects domain+ast+seam, not just the seam).
generate-seam: generate

# Freshness gate: the committed generated TS must match the Go source of truth
# (C13). Regenerate and fail if anything changed — i.e. someone edited the
# vocabulary/enums and forgot `make generate-seam`. This runs on the backend path
# (webkit-free), not check-app, because the person who causes drift is editing Go
# in internal/ast or internal/domain, and must catch it without the app toolchain.
check-generated: generate
	@git diff --exit-code -- frontend/src/_generated-types docs/data-dictionary.md >/dev/null 2>&1 || { \
		printf '\n\033[1;31m ✗ generated output is stale — run `make generate` and commit \033[0m\n\n'; \
		git --no-pager diff -- frontend/src/_generated-types docs/data-dictionary.md; \
		exit 1; \
	}

# --- backend, cheap → expensive ---

tidy-check-backend:
	go mod tidy -diff

build-backend:
	go build $(BACKEND_PKGS)

lint-backend:
	golangci-lint run $(BACKEND_PKGS)

vulncheck-backend:
	go run golang.org/x/vuln/cmd/govulncheck@latest $(BACKEND_PKGS)

test-backend:
	go test -race -coverprofile=coverage.out $(BACKEND_PKGS)

cover-backend: test-backend
	@grep -v -e '/cmd/dev/' -e '/internal/testutil/' coverage.out > coverage.filtered.out
	@go tool cover -func=coverage.filtered.out | tail -1
	@total=$$(go tool cover -func=coverage.filtered.out | tail -1 | awk '{gsub(/%/, ""); print $$3}'); \
	rm -f coverage.out coverage.filtered.out; \
	awk -v total="$$total" -v min="$(COVERAGE_MIN)" 'BEGIN { \
		if (total + 0 < min) { printf "coverage %.1f%% is below the %d%% gate\n", total, min; exit 1 } \
	}'

check-backend:
	@if $(MAKE) --no-print-directory tidy-check-backend check-generated build-backend lint-backend vulncheck-backend cover-backend; then \
		printf '\n\033[1;32m ✓ BACKEND PASSED \033[0m\n\n'; \
	else \
		printf '\n\033[1;31m ✗ BACKEND FAILED \033[0m\n\n'; \
		exit 1; \
	fi

# --- frontend ---

check-frontend:
	@if cd frontend && bun run check; then \
		printf '\n\033[1;32m ✓ FRONTEND PASSED \033[0m\n\n'; \
	else \
		printf '\n\033[1;31m ✗ FRONTEND FAILED \033[0m\n\n'; \
		exit 1; \
	fi

# --- app (the Wails composition root) ---

# The third check surface (impl/14 §2, decision 4): compile the root package the
# backend checks deliberately exclude, so a break in main.go/app.go or the bound
# seam services is caught. Needs the webkit toolchain (hence its own CI job with
# the gtk/webkit apt deps, kept off the backend job so isolation stays proven).
# Also re-runs the freshness gate so an app-path CI trigger verifies it too.
#
# main.go embeds frontend/dist (//go:embed), which must be non-empty to compile.
# This is a compile-check, not an asset-bundling check (that is `wails build`), so
# a stub file satisfies the embed when no real build is present — keeping the job
# Go+webkit only, no bun. A real dist (from wails build) is left untouched.
check-app:
	@mkdir -p frontend/dist
	@[ -n "$$(ls -A frontend/dist 2>/dev/null)" ] || touch frontend/dist/.gitkeep
	@if $(MAKE) --no-print-directory check-generated && go build -o /dev/null $(WEBKIT_TAGS) . ; then \
		printf '\n\033[1;32m ✓ APP PASSED \033[0m\n\n'; \
	else \
		printf '\n\033[1;31m ✗ APP FAILED \033[0m\n\n'; \
		exit 1; \
	fi
