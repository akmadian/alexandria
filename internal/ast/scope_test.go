package ast_test

import (
	"strings"
	"testing"

	"github.com/akmadian/alexandria/internal/ast"
)

func TestValidate_ScopeRules(t *testing.T) {
	cases := []struct {
		name    string
		scope   ast.Scope
		wantErr bool
	}{
		{"library bare", ast.Scope{Kind: ast.ScopeLibrary}, false},
		{"library with id", ast.Scope{Kind: ast.ScopeLibrary, ID: "x"}, true},
		{"library with source", ast.Scope{Kind: ast.ScopeLibrary, SourceID: "x"}, true},
		{"folder root", ast.Scope{Kind: ast.ScopeFolder, SourceID: "s1"}, false},
		{"folder with path", ast.Scope{Kind: ast.ScopeFolder, SourceID: "s1", Path: "2026/07"}, false},
		{"folder missing source", ast.Scope{Kind: ast.ScopeFolder, Path: "2026"}, true},
		{"folder with id", ast.Scope{Kind: ast.ScopeFolder, SourceID: "s1", ID: "x"}, true},
		{"folder absolute path", ast.Scope{Kind: ast.ScopeFolder, SourceID: "s1", Path: "/abs"}, true},
		{"folder trailing slash", ast.Scope{Kind: ast.ScopeFolder, SourceID: "s1", Path: "a/"}, true},
		{"collection", ast.Scope{Kind: ast.ScopeCollection, ID: "c1"}, false},
		{"collection missing id", ast.Scope{Kind: ast.ScopeCollection}, true},
		{"collection with source", ast.Scope{Kind: ast.ScopeCollection, ID: "c1", SourceID: "s1"}, true},
		{"tag", ast.Scope{Kind: ast.ScopeTag, ID: "t1"}, false},
		{"unknown kind", ast.Scope{Kind: "all"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := ast.Query{Version: ast.Version, Scope: &tc.scope}
			err := ast.Validate(query)
			if tc.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCompile_FolderScope(t *testing.T) {
	now := fixedNow()
	compile := func(t *testing.T, scope ast.Scope) ast.Statement {
		t.Helper()
		query := ast.Query{Version: ast.Version, Scope: &scope}
		statement, err := ast.CompileWhere(query, nil, now)
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		return statement
	}

	t.Run("root recursive is the whole source", func(t *testing.T) {
		statement := compile(t, ast.Scope{Kind: ast.ScopeFolder, SourceID: "s1", Recursive: true})
		if !strings.Contains(statement.SQL, "source_id = ?") || strings.Contains(statement.SQL, "relative_path") {
			t.Fatalf("unexpected SQL: %s", statement.SQL)
		}
	})

	t.Run("root non-recursive excludes subdirectories", func(t *testing.T) {
		statement := compile(t, ast.Scope{Kind: ast.ScopeFolder, SourceID: "s1"})
		if !strings.Contains(statement.SQL, "relative_path NOT LIKE '%/%'") {
			t.Fatalf("expected top-level-only clause: %s", statement.SQL)
		}
	})

	t.Run("path recursive matches the subtree", func(t *testing.T) {
		statement := compile(t, ast.Scope{Kind: ast.ScopeFolder, SourceID: "s1", Path: "2026/07", Recursive: true})
		if !strings.Contains(statement.SQL, "relative_path LIKE ? ESCAPE") {
			t.Fatalf("expected prefix LIKE: %s", statement.SQL)
		}
		if want := "2026/07/%"; statement.Args[1] != want {
			t.Fatalf("expected arg %q, got %v", want, statement.Args[1])
		}
	})

	t.Run("path non-recursive excludes deeper levels", func(t *testing.T) {
		statement := compile(t, ast.Scope{Kind: ast.ScopeFolder, SourceID: "s1", Path: "2026/07"})
		if !strings.Contains(statement.SQL, "NOT LIKE ? ESCAPE") {
			t.Fatalf("expected depth-limit clause: %s", statement.SQL)
		}
		if want := "2026/07/%/%"; statement.Args[2] != want {
			t.Fatalf("expected arg %q, got %v", want, statement.Args[2])
		}
	})

	t.Run("LIKE metacharacters in path are escaped", func(t *testing.T) {
		statement := compile(t, ast.Scope{Kind: ast.ScopeFolder, SourceID: "s1", Path: "100%_done", Recursive: true})
		if want := `100\%\_done/%`; statement.Args[1] != want {
			t.Fatalf("expected escaped arg %q, got %v", want, statement.Args[1])
		}
	})
}
