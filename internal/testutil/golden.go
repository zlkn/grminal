// Package testutil provides shared test helpers, notably a golden-file
// harness used across the VTE and render suites.
//
// Run `go test ./... -update` to (re)generate golden files, then review the
// diff before committing — golden files are reviewed like code.
package testutil

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// update rewrites golden files instead of comparing against them.
var update = flag.Bool("update", false, "update golden files")

// Golden compares got against the golden file testdata/<name>.golden.
//
// When -update is set it writes got to that path and passes. On mismatch it
// fails with a readable diff-friendly message; callers that produce binary
// output (e.g. PNGs) should prefer GoldenBytes which also dumps a .got file.
func Golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")

	if *update {
		writeGolden(t, path, got)
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run `go test -update` to create it)", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", path, want, got)
	}
}

// GoldenBytes is like Golden but, on mismatch, also writes got next to the
// golden file as <name>.got for out-of-band inspection (useful for images).
func GoldenBytes(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")

	if *update {
		writeGolden(t, path, got)
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run `go test -update` to create it)", path, err)
	}
	if !bytes.Equal(got, want) {
		gotPath := filepath.Join("testdata", name+".got")
		if werr := os.WriteFile(gotPath, got, 0o644); werr != nil {
			t.Logf("failed to write %s: %v", gotPath, werr)
		}
		t.Errorf("golden mismatch for %s (wrote actual output to %s)", path, gotPath)
	}
}

func writeGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for golden %s: %v", path, err)
	}
	if err := os.WriteFile(path, got, 0o644); err != nil {
		t.Fatalf("write golden %s: %v", path, err)
	}
	t.Logf("updated golden %s", path)
}
