package mps

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func writeTestZip(t *testing.T, path string, parts map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	// Deterministic order keeps the test output stable.
	names := make([]string, 0, len(parts))
	for name := range parts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create part %s: %v", name, err)
		}
		if _, err := w.Write([]byte(parts[name])); err != nil {
			t.Fatalf("write part %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
}

func readTestZipPart(t *testing.T, path, name string) (string, bool) {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open part %s: %v", name, err)
		}
		defer rc.Close()
		body, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read part %s: %v", name, err)
		}
		return string(body), true
	}
	return "", false
}

// A nil replacement means "remove this part". The bookkeeping that records
// which parts were already handled only covers parts that were present, so a
// nil entry for an absent part fell through to the "append a new part" loop and
// created an empty one.
func TestReplacePartsDoesNotCreateAPartForANilReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.zip")
	writeTestZip(t, path, map[string]string{"xl/workbook.xml": "<workbook/>"})

	if err := ReplaceParts(path, map[string][]byte{"xl/absent.xml": nil}); err != nil {
		t.Fatalf("ReplaceParts: %v", err)
	}
	if _, ok := readTestZipPart(t, path, "xl/absent.xml"); ok {
		t.Error("a nil replacement created an empty part")
	}
	if got, _ := readTestZipPart(t, path, "xl/workbook.xml"); got != "<workbook/>" {
		t.Errorf("the untouched part changed: %q", got)
	}
}

func TestReplacePartsDeletesRewritesAndAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.zip")
	writeTestZip(t, path, map[string]string{
		"xl/workbook.xml": "<workbook/>",
		"xl/old.xml":      "<old/>",
	})

	err := ReplaceParts(path, map[string][]byte{
		"xl/old.xml":  nil,
		"xl/new.xml":  []byte("<new/>"),
		"xl/kept.xml": nil,
	})
	if err != nil {
		t.Fatalf("ReplaceParts: %v", err)
	}

	if _, ok := readTestZipPart(t, path, "xl/old.xml"); ok {
		t.Error("an existing part was not deleted")
	}
	if got, ok := readTestZipPart(t, path, "xl/new.xml"); !ok || got != "<new/>" {
		t.Errorf("new part = %q, present = %v; want %q", got, ok, "<new/>")
	}
	if got, _ := readTestZipPart(t, path, "xl/workbook.xml"); got != "<workbook/>" {
		t.Errorf("the untouched part changed: %q", got)
	}
	if _, ok := readTestZipPart(t, path, "xl/kept.xml"); ok {
		t.Error("a nil replacement for an absent part created it")
	}
}

// Rewriting a part keeps the others' bytes, which is what preserves the VBA
// payload and every other part the export does not touch.
func TestReplacePartsRewritesInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.zip")
	writeTestZip(t, path, map[string]string{
		"xl/worksheets/sheet1.xml": "<sheet/>",
		"xl/vbaProject.bin":        "BINARY-PAYLOAD",
	})

	if err := ReplaceParts(path, map[string][]byte{"xl/worksheets/sheet1.xml": []byte("<sheet changed/>")}); err != nil {
		t.Fatalf("ReplaceParts: %v", err)
	}
	if got, _ := readTestZipPart(t, path, "xl/worksheets/sheet1.xml"); got != "<sheet changed/>" {
		t.Errorf("rewritten part = %q", got)
	}
	if got, _ := readTestZipPart(t, path, "xl/vbaProject.bin"); got != "BINARY-PAYLOAD" {
		t.Errorf("untouched binary part changed: %q", got)
	}
}
