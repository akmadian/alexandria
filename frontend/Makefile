.PHONY: lint test check

lint:
	bun run lint

test:
	bun run test

check:
	@if bun run check; then \
		printf '\n\033[1;32m ✓ PASSED \033[0m\n\n'; \
	else \
		printf '\n\033[1;31m ✗ FAILED \033[0m\n\n'; \
		exit 1; \
	fi
