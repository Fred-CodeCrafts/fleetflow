// Package arch holds architecture tests that guard the shape of the codebase.
//
// The routing core (internal/graph, internal/weights, internal/engine) is the
// hot path: it must never import an HTTP or database package, so it cannot reach
// the network or the database by accident (PRD US-5/US-12).
package arch

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// violation describes one banned import found in a package.
type violation struct {
	pkg        string // import path of the offending package
	file       string // file the import was found in
	importPath string // the banned import path
	kind       string // why the import is banned
}

func (v violation) String() string {
	return fmt.Sprintf("%s (%s) imports %q, but %s packages are banned on the routing core hot path",
		v.pkg, v.file, v.importPath, v.kind)
}

// bannedKind returns a short description of why path must not be imported by the
// routing core, or "" when the import is allowed.
func bannedKind(path string) string {
	switch {
	case path == "net/http" || strings.HasPrefix(path, "net/http/"):
		return "HTTP"
	case path == "database/sql" || strings.HasPrefix(path, "database/sql/"):
		return "SQL database"
	case strings.Contains(path, "postgres"), strings.Contains(path, "pgx"):
		return "Postgres driver"
	}
	return ""
}

// packageImportPath turns a package directory relative to this test package into
// its full import path, e.g. "../graph" -> "fleetflow/internal/graph".
func packageImportPath(dir string) string {
	return filepath.ToSlash(filepath.Clean(filepath.Join("fleetflow/internal/arch", dir)))
}

// scanDir parses the non-test .go files in dir and returns one violation per
// banned import found.
func scanDir(dir string) ([]violation, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	pkg := packageImportPath(dir)
	fset := token.NewFileSet()

	var found []violation
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
			continue // files the go tool ignores
		}

		path := filepath.Join(dir, name)
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		file, err := parser.ParseFile(fset, path, src, parser.ImportsOnly)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}

		for _, imp := range file.Imports {
			importPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return nil, fmt.Errorf("unquote import in %s: %w", path, err)
			}
			if kind := bannedKind(importPath); kind != "" {
				found = append(found, violation{pkg: pkg, file: path, importPath: importPath, kind: kind})
			}
		}
	}
	return found, nil
}

// TestRoutingCoreImports checks that the hot-path packages stay free of HTTP and
// database imports. The ../api package is the positive control: it imports
// net/http, so the same checker must report a violation for it, which proves the
// check is able to fail rather than always passing.
func TestRoutingCoreImports(t *testing.T) {
	tests := []struct {
		name        string
		dir         string
		wantBlocked bool // true when the package is expected to trip the checker
	}{
		{name: "graph", dir: "../graph"},
		{name: "weights", dir: "../weights"},
		{name: "engine", dir: "../engine"},
		{name: "api_control", dir: "../api", wantBlocked: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, err := scanDir(tt.dir)
			if err != nil {
				t.Fatalf("scanning %s: %v", tt.dir, err)
			}
			if tt.wantBlocked {
				if len(found) == 0 {
					t.Fatalf("expected %s to trip the checker, got no violations", tt.dir)
				}
				return
			}
			for _, v := range found {
				t.Errorf("banned import in %s: %v", packageImportPath(tt.dir), v)
			}
		})
	}
}

// TestCheckerDetectsNetHTTP is an explicit positive control: the checker must
// flag ../api for importing net/http.
func TestCheckerDetectsNetHTTP(t *testing.T) {
	found, err := scanDir("../api")
	if err != nil {
		t.Fatalf("scanning ../api: %v", err)
	}
	for _, v := range found {
		if v.importPath == "net/http" {
			return
		}
	}
	t.Fatalf("expected %s to be reported for importing %q, got violations: %v",
		packageImportPath("../api"), "net/http", found)
}
