package minirehs

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"testing"
)

func TestImagePackedMalformed(t *testing.T) {
	db, _ := compileImageTest(t, imageTestPatterns(), false)
	valid, err := db.(*database).marshalCompilationImage(true)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(5982))
	for i := 0; i < 2500; i++ {
		b := bytes.Clone(valid)
		off := imageHeaderSize + rng.Intn(len(b)-imageHeaderSize-4)
		b[off] ^= byte(1 + rng.Intn(255))
		imageFixCRC(b)
		if loaded, err := LoadImage(b); err == nil {
			_ = loaded.Close()
		}
	}
}

func TestImagePackedBlobExact(t *testing.T) {
	for _, pats := range [][]Pattern{imageTestPatterns(), syntheticImagePatterns(), {{ID: 3, Expr: `(?=x)`}}} {
		db, _ := compileImageTest(t, pats, false)
		m := db.(*database).imageMVS()
		packed, err := packImageBlob(m.blob, m.n)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := unpackImageBlob(packed, m.n)
		if err != nil || !bytes.Equal(raw, m.blob) {
			t.Fatal("native blob did not round-trip byte exactly", err)
		}
		t.Logf("patterns=%d dense_blob=%d packed_blob=%d", m.n, len(m.blob), len(packed))
		for i, n := range m.revNFAs {
			if n == nil {
				continue
			}
			original := encodeUnit(unitFromNFA(n, i, true))
			packed, err := packReverseImageUnit(original)
			if err != nil {
				t.Fatal(err)
			}
			again, err := unpackReverseImageUnit(packed)
			if err != nil || !bytes.Equal(original, again) {
				t.Fatal("reverse unit did not round-trip exactly", err)
			}
		}
	}
}

func TestImagePackedTables(t *testing.T) {
	rng := rand.New(rand.NewSource(153))
	w := newPackedTableWriter()
	var packed, want []byte
	var widths []int
	for i := 0; i < 300; i++ {
		width := 1 + rng.Intn(100)
		raw := make([]byte, width*8)
		switch i % 5 {
		case 0: // zero row
		case 1: // sparse individual bits
			for j := 0; j < 3; j++ {
				raw[rng.Intn(len(raw))] |= 1 << uint(rng.Intn(8))
			}
		case 2: // dense prefix, optionally zero padded
			_, _ = rng.Read(raw[:8])
		case 3: // sparse nonzero words
			for j := 0; j < 2; j++ {
				binary.LittleEndian.PutUint64(raw[rng.Intn(width)*8:], ^uint64(0))
			}
		default: // dense words
			_, _ = rng.Read(raw)
		}
		for repeat := 0; repeat < 2; repeat++ {
			packed = w.row(packed, raw)
			want = append(want, raw...)
			widths = append(widths, width)
		}
	}
	r := &imageReader{b: packed}
	d := &packedTableReader{r: r}
	var got []byte
	for _, width := range widths {
		got = d.words(got, 1, width)
	}
	if !r.done() || !bytes.Equal(got, want) {
		t.Fatal("bit row codec changed values", r.err)
	}
	var arrays [][]byte
	packed = nil
	for _, values := range [][]int32{nil, {0}, {-1, -1, -1, -1}, {1, 2, 3, 4, 5}, {2147483647, -2147483648, 100, -200, 0}} {
		arrays = append(arrays, putI32s(nil, values))
	}
	for repeat := 0; repeat < 2; repeat++ {
		for _, raw := range arrays {
			packed = w.intTable(packed, raw)
		}
	}
	r = &imageReader{b: packed}
	d = &packedTableReader{r: r}
	got = nil
	for repeat := 0; repeat < 2; repeat++ {
		for _, raw := range arrays {
			start := len(got)
			got = d.intTable(got, len(raw)/4)
			if r.err != nil || !bytes.Equal(got[start:], raw) {
				t.Fatal("integer table codec changed values", r.err)
			}
		}
	}
	if !r.done() {
		t.Fatal(r.err)
	}
}
