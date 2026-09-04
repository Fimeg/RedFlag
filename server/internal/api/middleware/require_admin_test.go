package middleware_test

// require_admin_test.go — Pre-fix tests for missing RequireAdmin middleware.
//
// BUG F-A3-13 LOW: RequireAdmin() middleware is referenced in main.go:601
// for security settings routes but was never implemented. The 7 security
// settings routes are permanently commented out because of this.
//
// Test 6.1 verifies that the middleware package exports a RequireAdmin symbol.
// Test 6.2 (build-tagged //go:build ignore) tests its behavior once implemented.
//
// Run: cd server && go test ./internal/api/middleware/... -v -run TestRequireAdmin

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test 6.1 — RequireAdmin middleware function exists in middleware package
//
// Category: FAIL-NOW / PASS-AFTER-FIX
//
// BUG F-A3-13: RequireAdmin() does not exist in the middleware package.
// Confirmed via grep: zero results for "RequireAdmin" in any .go file.
// 7 security settings routes in main.go:600-610 are commented out because
// of this missing middleware.
//
// This test scans the middleware package source files for a function named
// RequireAdmin. It does not attempt to call the function (which would fail
// to compile if it doesn't exist).
// ---------------------------------------------------------------------------

func TestRequireAdminMiddlewareExists(t *testing.T) {
	// Scan the middleware package directory for a RequireAdmin function
	middlewareDir := filepath.Join(".", "..", "..", "..", "internal", "api", "middleware")

	// Resolve relative to the test file location
	// For go test, the working directory is the package directory
	middlewareDir = "."

	entries, err := os.ReadDir(middlewareDir)
	if err != nil {
		t.Fatalf("failed to read middleware directory: %v", err)
	}

	found := false
	fset := token.NewFileSet()

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		node, err := parser.ParseFile(fset, entry.Name(), nil, parser.AllErrors)
		if err != nil {
			continue
		}

		for _, decl := range node.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if fn.Name.Name == "RequireAdmin" {
				found = true
				t.Logf("[INFO] [server] [middleware] RequireAdmin found in %s", entry.Name())
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		t.Errorf("[ERROR] [server] [middleware] RequireAdmin() function not found in middleware package.\n"+
			"BUG F-A3-13: RequireAdmin() is referenced in main.go:601 but never implemented.\n"+
			"7 security settings routes are permanently disabled as a result.\n"+
			"After fix: implement RequireAdmin() that checks UserClaims.Role == \"admin\".")
	}
}
