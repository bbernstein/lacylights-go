package gdtf

import (
	"archive/zip"
	"bytes"
	"errors"
	"strconv"
	"strings"
	"testing"
)

type zipEntry struct {
	name string
	data []byte
}

func buildZip(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(e.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func gdtfArchive(t *testing.T, desc string) []byte {
	return buildZip(t,
		zipEntry{"description.xml", []byte(desc)},
		zipEntry{"thumbnail.png", []byte("not really a png")},
		zipEntry{"models/3ds/body.3ds", []byte("model")},
	)
}

func extractBytes(data []byte, name string) ([]File, error) {
	return ExtractDescriptions(bytes.NewReader(data), int64(len(data)), name)
}

func TestExtractDescriptions_SingleGDTF(t *testing.T) {
	files, err := extractBytes(gdtfArchive(t, "<GDTF/>"), "Foo.gdtf")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].FileName != "Foo.gdtf" || files[0].DescriptionXML != "<GDTF/>" {
		t.Errorf("files = %+v", files)
	}
}

func TestExtractDescriptions_ZipOfGDTFs(t *testing.T) {
	outer := buildZip(t,
		zipEntry{"A.gdtf", gdtfArchive(t, "<A/>")},
		zipEntry{"dir/sub/B.GDTF", gdtfArchive(t, "<B/>")},
		zipEntry{"readme.txt", []byte("ignored")},
		zipEntry{"__MACOSX/._A.gdtf", []byte("resource fork")},
		zipEntry{"dir/._B.gdtf", []byte("resource fork")},
		zipEntry{"emptydir/", nil},
	)
	files, err := extractBytes(outer, "pack.zip")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range files {
		got[f.FileName] = f.DescriptionXML
	}
	if len(got) != 2 || got["pack.zip/A.gdtf"] != "<A/>" || got["pack.zip/dir/sub/B.GDTF"] != "<B/>" {
		t.Errorf("files = %v", got)
	}
}

func TestExtractDescriptions_Nested(t *testing.T) {
	inner := buildZip(t, zipEntry{"C.gdtf", gdtfArchive(t, "<C/>")})
	outer := buildZip(t, zipEntry{"inner.zip", inner})
	files, err := extractBytes(outer, "outer.zip")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].FileName != "outer.zip/inner.zip/C.gdtf" {
		t.Errorf("files = %+v", files)
	}

	// Depth 3 (zip in zip in zip holding a .gdtf) is too deep.
	deeper := buildZip(t, zipEntry{"outer.zip", outer})
	files, err = extractBytes(deeper, "deeper.zip")
	if !errors.Is(err, ErrNestingTooDeep) {
		t.Errorf("err = %v, want ErrNestingTooDeep", err)
	}
	if len(files) != 0 {
		t.Errorf("files = %+v", files)
	}
}

func TestExtractDescriptions_PartialFailure(t *testing.T) {
	outer := buildZip(t,
		zipEntry{"good.gdtf", gdtfArchive(t, "<G/>")},
		zipEntry{"nodesc.gdtf", buildZip(t, zipEntry{"thumbnail.png", []byte("x")})},
		zipEntry{"broken.gdtf", []byte("not a zip")},
	)
	files, err := extractBytes(outer, "mixed.zip")
	if len(files) != 1 || files[0].FileName != "mixed.zip/good.gdtf" {
		t.Errorf("files = %+v", files)
	}
	if !errors.Is(err, ErrNoDescription) || !errors.Is(err, ErrNotArchive) {
		t.Errorf("err = %v, want both ErrNoDescription and ErrNotArchive", err)
	}
	if !strings.Contains(err.Error(), "mixed.zip/nodesc.gdtf") {
		t.Errorf("err %v does not name the failed entry", err)
	}
}

func TestExtractDescriptions_Errors(t *testing.T) {
	t.Run("not a zip", func(t *testing.T) {
		if _, err := extractBytes([]byte("plain text"), "x.gdtf"); !errors.Is(err, ErrNotArchive) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("gdtf without description.xml", func(t *testing.T) {
		data := buildZip(t, zipEntry{"thumbnail.png", []byte("x")})
		if _, err := extractBytes(data, "x.gdtf"); !errors.Is(err, ErrNoDescription) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("zip without gdtf files", func(t *testing.T) {
		data := buildZip(t, zipEntry{"readme.txt", []byte("x")})
		if _, err := extractBytes(data, "x.zip"); !errors.Is(err, ErrNoDescription) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("renamed gdtf is still read", func(t *testing.T) {
		files, err := extractBytes(gdtfArchive(t, "<R/>"), "renamed.zip")
		if err != nil || len(files) != 1 || files[0].DescriptionXML != "<R/>" {
			t.Errorf("files = %+v, err = %v", files, err)
		}
	})
	t.Run("too many entries", func(t *testing.T) {
		entries := make([]zipEntry, MaxArchiveEntries+1)
		for i := range entries {
			entries[i] = zipEntry{name: "f" + strconv.Itoa(i) + ".txt"}
		}
		if _, err := extractBytes(buildZip(t, entries...), "many.zip"); !errors.Is(err, ErrTooManyEntries) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("description too large", func(t *testing.T) {
		big := bytes.Repeat([]byte(" "), MaxDescriptionBytes+1)
		if _, err := extractBytes(gdtfArchive(t, string(big)), "big.gdtf"); !errors.Is(err, ErrDescriptionTooLarge) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("description exactly at the limit", func(t *testing.T) {
		exact := bytes.Repeat([]byte(" "), MaxDescriptionBytes)
		files, err := extractBytes(gdtfArchive(t, string(exact)), "exact.gdtf")
		if err != nil || len(files) != 1 || len(files[0].DescriptionXML) != MaxDescriptionBytes {
			t.Errorf("err = %v", err)
		}
	})
}

// TestReadLimited_HeaderLies checks that the size limit is enforced on the
// bytes actually read, not only on the (forgeable) header size.
func TestReadLimited_HeaderLies(t *testing.T) {
	data := buildZip(t, zipEntry{"description.xml", bytes.Repeat([]byte("x"), 100)})
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	f := zr.File[0]
	if _, err := readLimited(f, 10); !errors.Is(err, errLimit) {
		t.Errorf("err = %v, want errLimit", err)
	}
	got, err := readLimited(f, 100)
	if err != nil || len(got) != 100 {
		t.Errorf("read %d bytes, err %v", len(got), err)
	}
}

func TestExtractDescriptions_TotalFileBudget(t *testing.T) {
	one := gdtfArchive(t, "<GDTF/>")
	inner := func() []byte {
		entries := make([]zipEntry, MaxTotalFiles/2+1)
		for i := range entries {
			entries[i] = zipEntry{"F" + strconv.Itoa(i) + ".gdtf", one}
		}
		return buildZip(t, entries...)
	}
	outer := buildZip(t,
		zipEntry{"a.zip", inner()},
		zipEntry{"b.zip", inner()},
		zipEntry{"c.zip", inner()}, // never opened: the budget is spent first
	)
	files, err := extractBytes(outer, "all.zip")
	if !errors.Is(err, ErrExtractionTooLarge) {
		t.Fatalf("err = %v, want ErrExtractionTooLarge", err)
	}
	if len(files) != MaxTotalFiles {
		t.Errorf("files = %d, want %d", len(files), MaxTotalFiles)
	}
}

func TestExtractBudget_Bytes(t *testing.T) {
	b := &extractBudget{}
	if !b.take(MaxTotalDescriptionBytes - 10) {
		t.Fatal("first description should fit")
	}
	if b.take(11) {
		t.Error("description past the byte budget should not fit")
	}
	if !b.take(10) {
		t.Error("description that exactly fills the budget should fit")
	}
}
