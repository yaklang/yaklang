package minirehs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestImageStorageVariants(t *testing.T) {
	pats := imageTestPatterns()
	original, compressedPacked := compileImageTest(t, pats, false)
	d := original.(*database)
	dense, err := d.marshalUncompressedImage()
	if err != nil {
		t.Fatal(err)
	}
	packed, err := d.marshalCompilationImage(true)
	if err != nil {
		t.Fatal(err)
	}
	compressedDense, err := compressCompilationImage(dense)
	if err != nil {
		t.Fatal(err)
	}
	var compiles int
	imageCompileHook = func() { compiles++ }
	defer func() { imageCompileHook = nil }()
	for _, v := range []struct {
		name string
		data []byte
	}{{"dense", dense}, {"packed", packed}, {"compressed-dense", compressedDense}, {"compressed-packed", compressedPacked}} {
		t.Run(v.name, func(t *testing.T) {
			loaded, err := LoadImage(v.data)
			if err != nil {
				t.Fatal(err)
			}
			defer loaded.Close()
			if !bytes.Equal(d.imageMVS().blob, loaded.(*database).imageMVS().blob) || !reflect.DeepEqual(original.Info(), loaded.Info()) {
				t.Fatal("storage encoding changed runtime state")
			}
			in := []byte("abc12xyz aa helloworld password42 你好123")
			want, e := imageMatches(original, in)
			got, e2 := imageMatches(loaded, in)
			if e != nil || e2 != nil || !reflect.DeepEqual(want, got) {
				t.Fatal("storage encoding changed matches", e, e2)
			}
			path := filepath.Join(t.TempDir(), "cache.rehs")
			if err := os.WriteFile(path, v.data, 0644); err != nil {
				t.Fatal(err)
			}
			cached, err := LoadOrCompile(path, pats, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
			if err != nil {
				t.Fatal(err)
			}
			defer cached.Close()
			again, err := cached.MarshalImage()
			if err != nil || !bytes.Equal(again, v.data) || compiles != 0 {
				t.Fatal("compatible cache was recompiled or changed", compiles, err)
			}
		})
	}
}

func TestImageRawCompatibility(t *testing.T) {
	original, raw := compileRawImageTest(t, imageTestPatterns(), true)
	loaded, err := LoadImage(raw)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Close()
	again, err := loaded.MarshalImage()
	if err != nil || !bytes.Equal(raw, again) {
		t.Fatal("legacy raw image was not preserved", err)
	}
	clear(raw)
	want, e := imageMatches(original, []byte("abc12xyz aa helloworld password42"))
	got, e2 := imageMatches(loaded, []byte("abc12xyz aa helloworld password42"))
	if e != nil || e2 != nil || !reflect.DeepEqual(want, got) {
		t.Fatal("raw image ownership or semantics differ", e, e2, want, got)
	}
	clear(again)
	third, err := loaded.MarshalImage()
	if err != nil || bytes.Equal(again, third) {
		t.Fatal("export buffer aliases database storage", err)
	}
}

func TestImageCompressedValidation(t *testing.T) {
	db, valid := compileImageTest(t, imageTestPatterns(), false)
	raw, err := db.(*database).marshalCompilationImage(true)
	if err != nil {
		t.Fatal(err)
	}
	h, err := readImageHeader(valid)
	if err != nil || !h.compressed || h.rawSize != len(raw) || len(valid) >= len(raw)/2 {
		t.Fatal("image was not compressed", h, len(valid), len(raw), err)
	}
	for n := 0; n < len(valid); n++ {
		if _, err := LoadImage(valid[:n]); !errors.Is(err, ErrCorruptImage) {
			t.Fatalf("compressed truncate=%d: %v", n, err)
		}
	}
	envelope := func(payload []byte) []byte {
		w, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1), zstd.WithWindowSize(imageCompressionWindow))
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close()
		b := bytes.Clone(valid[:imageHeaderSize])
		b = putU32(b, uint32(len(payload)))
		b = w.EncodeAll(payload, b)
		return putU32(b, crc32.ChecksumIEEE(b))
	}
	badInnerCRC := bytes.Clone(raw)
	badInnerCRC[len(badInnerCRC)-1] ^= 1
	cases := map[string][]byte{
		"inner-checksum":  envelope(badInnerCRC),
		"nested-envelope": envelope(valid),
		"trailing-data":   append(bytes.Clone(valid[:len(valid)-4]), 1, 2, 3, 4, 0, 0, 0, 0),
		"truncated-frame": append(bytes.Clone(valid[:len(valid)-6]), 0, 0, 0, 0),
	}
	for _, size := range []uint32{0, uint32(len(raw) - 1), uint32(len(raw) + 1), 0x7fffffff, 0xffffffff} {
		b := bytes.Clone(valid)
		binary.LittleEndian.PutUint32(b[imageHeaderSize:], size)
		cases[fmt.Sprintf("declared-size-%d", size)] = b
	}
	for _, off := range []int{12, 16, 20, 28, imageHeaderSize + 4} {
		b := bytes.Clone(valid)
		b[off] ^= 1
		cases[fmt.Sprintf("mismatched-header-%d", off)] = b
	}
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			imageFixCRC(b)
			if loaded, err := LoadImage(b); !errors.Is(err, ErrCorruptImage) {
				if loaded != nil {
					_ = loaded.Close()
				}
				t.Fatal("malformed compressed image accepted", err)
			}
		})
	}
}
