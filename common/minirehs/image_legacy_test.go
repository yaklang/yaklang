package minirehs

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestImageLegacyV1AndCacheRefresh(t *testing.T) {
	legacy, err := os.ReadFile("testdata/compile-image-v1.rehs")
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(legacy[4:]) != 1 {
		t.Fatal("fixture must come from the original v1 compiler")
	}
	compiles, parses := 0, 0
	imageCompileHook = func() { compiles++ }
	imageParseHook = func() { parses++ }
	defer func() { imageCompileHook, imageParseHook = nil, nil }()
	db, err := LoadImage(legacy)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if compiles != 0 || parses != 0 {
		t.Fatal("legacy load invoked the compiler")
	}
	again, err := db.MarshalImage()
	if err != nil || !bytes.Equal(again, legacy) {
		t.Fatal("legacy re-export changed", err)
	}
	got, err := imageMatches(db, []byte("aa"))
	if err != nil || len(got) != 1 || got[0].ID != 6 {
		t.Fatal("legacy PCRE2 rule changed", got, err)
	}
	var pats []Pattern
	patternJSON, err := os.ReadFile("examples/embedded/patterns.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(patternJSON, &pats); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cache.rehs")
	if err := os.WriteFile(path, legacy, 0644); err != nil {
		t.Fatal(err)
	}
	opts := []Option{WithBackend(BackendMVS), WithReportLocation(false), WithMinLiteralLen(2), WithLogger(silentLogger{})}
	for i := 0; i < 2; i++ {
		updated, err := LoadOrCompile(path, pats, opts...)
		if err != nil {
			t.Fatal(err)
		}
		_ = updated.Close()
		if compiles != 1 {
			t.Fatalf("cache should rebuild once then hit: iteration=%d compiles=%d", i, compiles)
		}
	}
	current, err := os.ReadFile(path)
	if err != nil || binary.LittleEndian.Uint32(current[4:]) != imageVersion {
		t.Fatal("cache did not upgrade", err)
	}
}

func TestImageEnvelopeVersionMismatch(t *testing.T) {
	_, raw := compileRawImageTest(t, imageTestPatterns(), false)
	encoded, err := compressCompilationImage(raw)
	if err != nil {
		t.Fatal(err)
	}
	h, err := readImageHeader(encoded)
	if err != nil || !h.compressed {
		t.Fatal("test needs a compressed image", err)
	}
	binary.LittleEndian.PutUint32(encoded[4:], 1)
	imageFixCRC(encoded)
	if _, err := LoadImage(encoded); !errors.Is(err, ErrCorruptImage) {
		t.Fatal("outer/inner version mismatch accepted", err)
	}
}
