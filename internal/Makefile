.PHONY: lint test cover check

lint:
	cd .. && golangci-lint run ./...

test:
	cd .. && go test -race -coverprofile=coverage.out ./...

cover: test
	cd .. && go tool cover -func=coverage.out | tail -1
	@rm -f ../coverage.out

check:
	@if $(MAKE) --no-print-directory lint cover; then \
		printf '\n\033[1;32m ✓ PASSED \033[0m\n\n'; \
	else \
		printf '\n\033[1;31m ✗ FAILED \033[0m\n\n'; \
		exit 1; \
	fi
