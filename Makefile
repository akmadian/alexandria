# All checks live here — Go only operates at the module root, so there is no
# pretending with per-directory Makefiles. Same checks CI runs: use
# `make check` (or `make check-backend`) before pushing.
.PHONY: check check-backend check-frontend tidy-check-backend build-backend lint-backend vulncheck-backend test-backend cover-backend

# Ratchet up as areas gain tests; never down. Excludes cmd/dev + testutil (wiring
# and test support by design). Measured 74.0% at gate creation (2026-07-09).
COVERAGE_MIN := 70

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

# --- backend, cheap → expensive ---

tidy-check-backend:
	go mod tidy -diff

build-backend:
	go build ./...

lint-backend:
	golangci-lint run ./...

vulncheck-backend:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

test-backend:
	go test -race -coverprofile=coverage.out ./...

cover-backend: test-backend
	@grep -v -e '/cmd/dev/' -e '/internal/testutil/' coverage.out > coverage.filtered.out
	@go tool cover -func=coverage.filtered.out | tail -1
	@total=$$(go tool cover -func=coverage.filtered.out | tail -1 | awk '{gsub(/%/, ""); print $$3}'); \
	rm -f coverage.out coverage.filtered.out; \
	awk -v total="$$total" -v min="$(COVERAGE_MIN)" 'BEGIN { \
		if (total + 0 < min) { printf "coverage %.1f%% is below the %d%% gate\n", total, min; exit 1 } \
	}'

check-backend:
	@if $(MAKE) --no-print-directory tidy-check-backend build-backend lint-backend vulncheck-backend cover-backend; then \
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
