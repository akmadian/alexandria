.PHONY: check check-backend check-frontend

# Same checks CI runs — use `make check` before pushing.

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

check-backend:
	$(MAKE) -C internal check

check-frontend:
	$(MAKE) -C frontend check
