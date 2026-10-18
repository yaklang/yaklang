package minirehs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func imageTestPatterns() []Pattern {
	exprs := []string{`abc[0-9]{2,4}xyz`, `token=\w+`, `[\w.-]+\.example\.com`, `foo.*bar`,
		`\b(?:ab\b|cd$)`, `[0-9]{3}`, `^[a-z]{2}$`, `^(a)\1$`, `hello(?=world)`,
		`a*`, `(?=x)`, `你好[0-9]+`, `.`, `(?m)^` + strings.Repeat(`[ab]`, 70) + `$`,
		mvsJSONExpr, mvsMACExpr, `(?:\b|^)ab(?:\b|$)`, strings.Repeat(`[ab]`, 75) + `xyz`}
	pats := make([]Pattern, 0, len(exprs)+1)
	for i, s := range exprs {
		pats = append(pats, Pattern{ID: PatternID(i*7 + 3), Expr: s})
	}
	return append(pats, Pattern{ID: 999, Expr: `PASSWORD[0-9]{2}`, Flags: FlagCaseless | FlagDotAll})
}

func compileImageTest(tb testing.TB, pats []Pattern, loc bool) (Database, []byte) {
	tb.Helper()
	db, err := Compile(pats, WithBackend(BackendMVS), WithReportLocation(loc), WithLogger(silentLogger{}))
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = db.Close() })
	data, err := db.MarshalImage()
	if err != nil {
		tb.Fatal(err)
	}
	return db, data
}

func compileRawImageTest(tb testing.TB, pats []Pattern, loc bool) (Database, []byte) {
	tb.Helper()
	db, _ := compileImageTest(tb, pats, loc)
	raw, err := db.(*database).marshalUncompressedImage()
	if err != nil {
		tb.Fatal(err)
	}
	return db, raw
}

func imageMatches(db Database, in []byte) ([]Match, error) {
	var out []Match
	err := db.Scan(in, nil, func(m Match) bool { out = append(out, m); return true })
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out, err
}

func TestImageRoundTrip(t *testing.T) {
	for _, loc := range []bool{false, true} {
		t.Run(map[bool]string{false: "existence", true: "location"}[loc], func(t *testing.T) {
			original, data := compileImageTest(t, imageTestPatterns(), loc)
			_, second := compileImageTest(t, imageTestPatterns(), loc)
			if !bytes.Equal(data, second) {
				t.Fatal("independent compilations are not deterministic")
			}
			loaded, err := LoadImage(data)
			if err != nil {
				t.Fatal(err)
			}
			defer loaded.Close()
			again, err := loaded.MarshalImage()
			if err != nil || !bytes.Equal(data, again) {
				t.Fatalf("remarshal differs: %v", err)
			}
			if !reflect.DeepEqual(original.Info(), loaded.Info()) {
				t.Fatalf("info differs: %+v / %+v", original.Info(), loaded.Info())
			}
			m := loaded.(*database).imageMVS()
			originalM := original.(*database).imageMVS()
			if m.mergedNecFactor != originalM.mergedNecFactor {
				t.Fatal("merged Go precondition differs")
			}
			for i := range m.all {
				if getNecFactor(m.assertNecFactor, i) != getNecFactor(originalM.assertNecFactor, i) {
					t.Fatal("assert Go precondition differs", i)
				}
			}
			if m.hasBiAnchor != original.(*database).imageMVS().hasBiAnchor || !m.hasBiAnchor {
				t.Fatal("reverse NFA route not exercised")
			}
			if (m.kernel != nil) != mvsKernelAvailable() {
				t.Fatal("wrong kernel for build")
			}
			// All retained storage must belong to the database, not the caller.
			clear(data)
			again2, e := loaded.MarshalImage()
			if e != nil || !bytes.Equal(again, again2) {
				t.Fatal("input buffer retained")
			}
			inputs := [][]byte{nil, {}, []byte("abc12xyz token=abc a.example.com fooXYZbar ab cd\naa\nhelloworld password42"),
				[]byte("aa"), []byte("x"), []byte("你好123"), {0x80}, []byte(strings.Repeat("a", 75) + "xyz"),
				[]byte("{\"key\":123} 00:12:ab:34:56:cd"), []byte("\n" + strings.Repeat("b", 70) + "\n")}
			rng := rand.New(rand.NewSource(47))
			for i := 0; i < 150; i++ {
				b := make([]byte, rng.Intn(120))
				_, _ = rng.Read(b)
				inputs = append(inputs, b)
			}
			for _, in := range inputs {
				want, e := imageMatches(original, in)
				got, e2 := imageMatches(loaded, in)
				if e != nil || e2 != nil || !reflect.DeepEqual(want, got) {
					t.Fatalf("input=%q want=%v got=%v errors=%v/%v", in, want, got, e, e2)
				}
			}
			records := [][]byte{bytes.Repeat([]byte("abc12xyz password42 "), 1500), []byte("你好123"), []byte("aa")}
			var want, got []batchMatch
			_ = original.ScanBatch(records, nil, func(i int, m Match) bool { want = append(want, batchMatch{record: i, match: m}); return true })
			_ = loaded.ScanBatch(records, nil, func(i int, m Match) bool { got = append(got, batchMatch{record: i, match: m}); return true })
			if !reflect.DeepEqual(want, got) {
				t.Fatal("batch matches differ")
			}
			calls := 0
			if err = loaded.Scan([]byte("abc12xyz"), nil, func(Match) bool { calls++; return false }); err != nil || calls != 1 {
				t.Fatal("early stop", calls, err)
			}
		})
	}
}

func TestImageLoadDoesNotCompile(t *testing.T) {
	_, data := compileImageTest(t, imageTestPatterns(), false)
	var parses, nfas, compiles int
	imageParseHook = func() { parses++ }
	imageNFAHook = func() { nfas++ }
	imageCompileHook = func() { compiles++ }
	defer func() { imageParseHook = nil; imageNFAHook = nil; imageCompileHook = nil }()
	db, err := LoadImage(data)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if parses != 0 || nfas != 0 || compiles != 0 {
		t.Fatalf("load compiled: parse=%d NFA=%d compile=%d", parses, nfas, compiles)
	}
	for _, p := range db.(*database).imageMVS().all {
		switch v := p.v.(type) {
		case *re2Verifier:
			if v.re != nil {
				t.Fatal("eager RE2 verifier")
			}
		case *regexp2Verifier:
			if v.yak != nil {
				t.Fatal("eager regexp2 verifier")
			}
		}
	}
	// A lean existence hit must never need even its lazy verifier.
	_, lean := compileImageTest(t, []Pattern{{ID: 1, Expr: `abc[0-9]{2}`}}, false)
	db2, err := LoadImage(lean)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	_, _ = imageMatches(db2, []byte("abc12"))
	if db2.(*database).imageMVS().all[0].v.(*re2Verifier).re != nil {
		t.Fatal("NFA existence compiled a verifier")
	}
}

func TestImageConcurrentLazyScan(t *testing.T) {
	for _, loc := range []bool{false, true} {
		original, data := compileImageTest(t, imageTestPatterns(), loc)
		in := []byte("aa ab helloworld password42 你好123 abc12xyz")
		want, err := imageMatches(original, in)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := LoadImage(data)
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 20; j++ {
					got, e := imageMatches(loaded, in)
					if e != nil || !reflect.DeepEqual(want, got) {
						t.Errorf("concurrent scan: %v %v", got, e)
						return
					}
				}
			}()
		}
		wg.Wait()
		_ = loaded.Close()
	}
}

func TestImageMITMCorpus(t *testing.T) {
	pats, _ := compilableMITMPatterns(t)
	records, _ := loadCorpus(t)
	for _, loc := range []bool{false, true} {
		original, data := compileImageTest(t, pats, loc)
		loaded, err := LoadImage(data)
		if err != nil {
			t.Fatal(err)
		}
		for i, in := range records {
			want, e := imageMatches(original, in)
			got, e2 := imageMatches(loaded, in)
			if e != nil || e2 != nil || !reflect.DeepEqual(want, got) {
				t.Fatalf("record=%d loc=%v mismatch %v / %v: %v %v", i, loc, want, got, e, e2)
			}
		}
		again, err := loaded.MarshalImage()
		_ = loaded.Close()
		if err != nil || !bytes.Equal(data, again) {
			t.Fatalf("MITM remarshal: %v", err)
		}
	}
}

func TestImageGetRequest(t *testing.T) {
	if testing.Short() {
		t.Skip("large rule set; covered by the full image suite")
	}
	pats := getRequestImagePatterns(t)
	compiled, data := compileImageTest(t, pats, false)
	_, again := compileImageTest(t, pats, false)
	if !bytes.Equal(data, again) {
		t.Fatal("GetRequest image is not deterministic")
	}
	loaded, err := LoadImage(data)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Close()
	if !reflect.DeepEqual(compiled.Info(), loaded.Info()) {
		t.Fatal("GetRequest metadata differs")
	}
	if !bytes.Equal(compiled.(*database).imageMVS().blob, loaded.(*database).imageMVS().blob) {
		t.Fatal("GetRequest native blob was not restored exactly")
	}
	inputs := [][]byte{[]byte("HTTP/1.1 200 OK\r\nServer: nginx/1.24.0\r\n\r\n<html>ok</html>"), []byte("SSH-2.0-OpenSSH_9.0\r\n"), {0x80}, []byte("HTTP/1.0 404 Not Found\r\n\r\n")}
	for _, in := range inputs {
		want, e := imageMatches(compiled, in)
		got, e2 := imageMatches(loaded, in)
		if e != nil || e2 != nil || !reflect.DeepEqual(want, got) {
			t.Fatal("GetRequest scan differs", want, got, e, e2)
		}
	}
	raw, err := compiled.(*database).marshalUncompressedImage()
	if err != nil {
		t.Fatal(err)
	}
	packed, err := compiled.(*database).marshalCompilationImage(true)
	if err != nil {
		t.Fatal(err)
	}
	m := compiled.(*database).imageMVS()
	revBytes := 0
	for i, n := range m.revNFAs {
		if n != nil {
			revBytes += len(encodeUnit(unitFromNFA(n, i, true)))
		}
	}
	packedBlob, err := packImageBlob(m.blob, m.n)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("patterns=%d compressed_image=%d packed_image=%d dense_image=%d dense_native_blob=%d packed_native_blob=%d reverse_units=%d dense_metadata=%d", len(pats), len(data), len(packed), len(raw), len(m.blob), len(packedBlob), revBytes, len(raw)-len(m.blob)-revBytes)
}

func imageFixCRC(b []byte) {
	binary.LittleEndian.PutUint32(b[len(b)-4:], crc32.ChecksumIEEE(b[:len(b)-4]))
}

func TestImageRejectMalformed(t *testing.T) {
	_, valid := compileRawImageTest(t, []Pattern{{ID: 1, Expr: `\babc\b`}}, false)
	for _, off := range []int{0, 4, 8, 12, 16, 28, len(valid) - 1} {
		b := bytes.Clone(valid)
		b[off] ^= 0xff
		want := ErrCorruptImage
		if off == 4 || off == 8 || off == 12 {
			imageFixCRC(b)
			want = ErrIncompatibleImage
		}
		if _, err := LoadImage(b); !errors.Is(err, want) {
			t.Fatalf("offset=%d got=%v want=%v", off, err, want)
		}
	}
	for n := 0; n < len(valid); n++ {
		if _, err := LoadImage(valid[:n]); !errors.Is(err, ErrCorruptImage) {
			t.Fatalf("truncate=%d err=%v", n, err)
		}
	}
	// Mutate lengths, indices, scalar tables and extension payloads with a valid
	// checksum, exercising the decoder rather than just its CRC guard.
	rng := rand.New(rand.NewSource(398))
	for i := 0; i < 2000; i++ {
		b := bytes.Clone(valid)
		off := imageHeaderSize + rng.Intn(len(b)-imageHeaderSize-4)
		b[off] ^= byte(1 + rng.Intn(255))
		imageFixCRC(b)
		if db, err := LoadImage(b); err == nil {
			_, _ = imageMatches(db, []byte("abc xyz"))
			_ = db.Close()
		}
	}
	_, blob := compileRawImageTest(t, imageTestPatterns(), false)
	start := bytes.Index(blob, []byte("MVS1"))
	if start < 0 {
		t.Fatal("no blob")
	}
	for _, off := range []int{0, 4, 8, 12, 16, 20} {
		b := bytes.Clone(blob)
		binary.LittleEndian.PutUint32(b[start+off:], 0x7fffffff)
		imageFixCRC(b)
		if _, err := LoadImage(b); !errors.Is(err, ErrCorruptImage) {
			t.Fatalf("blob offset=%d err=%v", off, err)
		}
	}
	b := bytes.Clone(blob)
	binary.LittleEndian.PutUint32(b[start+16:], 0)
	binary.LittleEndian.PutUint32(b[start+12:], 0xffffffff)
	imageFixCRC(b)
	if _, err := LoadImage(b); !errors.Is(err, ErrCorruptImage) {
		t.Fatal("slot without a unit", err)
	}
}

func TestImageCacheProcess(t *testing.T) {
	if path := os.Getenv("MINIREHS_CACHE_PROCESS_PATH"); path != "" {
		imageCompileHook = func() {
			f, err := os.OpenFile(path+".compiles", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.Write([]byte{1})
			_ = f.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
		db, err := LoadOrCompile(path, syntheticImagePatterns(), WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatal(err)
		}
		_ = db.Close()
		return
	}
	path := filepath.Join(t.TempDir(), "shared.rehs")
	var cmds []*exec.Cmd
	for i := 0; i < 4; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestImageCacheProcess$")
		cmd.Env = append(os.Environ(), "MINIREHS_CACHE_PROCESS_PATH="+path)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cmds = append(cmds, cmd)
	}
	for _, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	count, err := os.ReadFile(path + ".compiles")
	if err != nil || len(count) != 1 {
		t.Fatalf("cross-process compiles=%d err=%v", len(count), err)
	}
}

func TestImageCacheAtomicReplace(t *testing.T) {
	_, a := compileImageTest(t, []Pattern{{ID: 1, Expr: "abc"}}, false)
	_, b := compileImageTest(t, []Pattern{{ID: 2, Expr: "xyz"}}, false)
	path := filepath.Join(t.TempDir(), "atomic.rehs")
	if err := writeImageCache(path, a); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	done := make(chan struct{})
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				got, err := readImageFile(path)
				if err != nil || !bytes.Equal(got, a) && !bytes.Equal(got, b) {
					t.Errorf("reader observed a partial replacement: %v", err)
					return
				}
			}
		}()
	}
	for i := 0; i < 16; i++ {
		data := a
		if i%2 == 0 {
			data = b
		}
		if err := writeImageCache(path, data); err != nil {
			t.Error(err)
			break
		}
	}
	close(done)
	wg.Wait()
}

func TestImageUnsupportedBackend(t *testing.T) {
	for _, kind := range []BackendKind{Auto, BackendEngine, BackendStdlib} {
		path := filepath.Join(t.TempDir(), "absent", "cache.rehs")
		db, err := LoadOrCompile(path, []Pattern{{Expr: "abc"}}, WithBackend(kind), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.MarshalImage()
		_ = db.Close()
		if !errors.Is(err, ErrUnsupportedImage) {
			t.Fatal(err)
		}
		if _, err = os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("non-MVS accessed cache", err)
		}
	}
}

func TestImageCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "rules.rehs")
	pats := []Pattern{{ID: 12, Expr: `abc[0-9]{2}`}, {ID: 99, Expr: `^xyz$`}}
	opts := []Option{WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{})}
	var compiles int32
	imageCompileHook = func() { atomic.AddInt32(&compiles, 1) }
	defer func() { imageCompileHook = nil }()
	load := func() {
		t.Helper()
		db, err := LoadOrCompile(path, pats, opts...)
		if err != nil {
			t.Fatal(err)
		}
		_ = db.Close()
	}
	load()
	load()
	if compiles != 1 {
		t.Fatalf("cache hit recompiled: %d", compiles)
	}
	pats[0].Expr = `abc[0-9]{3}`
	load()
	if compiles != 2 {
		t.Fatal("changed expression not invalidated")
	}
	pats[0].Flags = FlagCaseless
	load()
	if compiles != 3 {
		t.Fatal("changed flags not invalidated")
	}
	pats[0], pats[1] = pats[1], pats[0]
	load()
	if compiles != 4 {
		t.Fatal("changed order not invalidated")
	}
	opts = append(opts, WithMinLiteralLen(4))
	load()
	if compiles != 5 {
		t.Fatal("changed options not invalidated")
	}
	if err := os.WriteFile(path, []byte("partial"), 0644); err != nil {
		t.Fatal(err)
	}
	load()
	if compiles != 6 {
		t.Fatal("corrupt file not replaced")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db, err := LoadOrCompile(path, pats, opts...)
			if err != nil {
				t.Error(err)
				return
			}
			_ = db.Close()
		}()
	}
	wg.Wait()
	if compiles != 7 {
		t.Fatalf("concurrent misses compiled %d times", compiles-6)
	}
	files, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(files) != 1 {
		t.Fatalf("temporary files leaked: %v %v", files, err)
	}
	if _, err := LoadOrCompile("", pats, opts...); err == nil {
		t.Fatal("empty cache path accepted")
	}
	if _, err := LoadOrCompile(filepath.Dir(path), pats, opts...); err == nil {
		t.Fatal("directory accepted as file")
	}
	unlock, err := lockImageCache(path+".lock", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if _, err = lockImageCache(path+".lock", time.Millisecond); err == nil {
		t.Fatal("lock timeout ignored")
	}
}

func FuzzLoadImage(f *testing.F) {
	f.Add([]byte("REHS"))
	db, valid := compileImageTest(f, imageTestPatterns(), false)
	f.Add(valid)
	raw, err := db.(*database).marshalUncompressedImage()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(raw)
	packed, err := db.(*database).marshalCompilationImage(true)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(packed)
	f.Fuzz(func(t *testing.T, b []byte) {
		// Recompute a valid CRC for some inputs so mutations reach unit decoding.
		if len(b) > imageHeaderSize+4 && len(b) < 1<<20 {
			b = bytes.Clone(b)
			imageFixCRC(b)
		}
		if db, err := LoadImage(b); err == nil {
			_, _ = imageMatches(db, []byte("aa abc12xyz helloworld"))
			_, _ = db.MarshalImage()
			_ = db.Close()
		}
	})
}
