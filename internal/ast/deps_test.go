package ast_test

import (
	"os/exec"
	"strings"
	"testing"
)

// Dependency check: internal/ast imports only internal/domain and stdlib.
func TestPackageDependencies(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "github.com/akmadian/alexandria/internal/ast")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("go list failed: %v\n%s", err, out)
	}

	for _, dep := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if dep == "" {
			continue
		}
		if strings.HasPrefix(dep, "github.com/akmadian/alexandria/internal/") {
			allowed := dep == "github.com/akmadian/alexandria/internal/ast" ||
				dep == "github.com/akmadian/alexandria/internal/domain"
			if !allowed {
				t.Errorf("unexpected internal dependency: %s", dep)
			}
		}
	}
}
