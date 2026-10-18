//go:build cgo

package minirehs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// State zero represents no consumed positions. The transition injects first
// before consuming the byte, as the original NFA does on every unanchored step.
func runDFAForTest(d *mvsDFA, data []byte) bool {
	state := 0
	for _, b := range data {
		state = int(d.next[state*256+int(b)])
		if state < 0 {
			state = 0
			continue
		}
		if d.accept[state] != 0 {
			return true
		}
	}
	return false
}

func TestDFACanonicalInitialState(t *testing.T) {
	for _, expr := range []string{`[ab][cd]`, `[ab]{2}`, `[ab]{1,3}c`, `a*a?b`, `(?:ab|ba)+c`, `a`, `[ab]{63}`, `[ab]{64}`} {
		t.Run(expr, func(t *testing.T) {
			n := buildNFAFor(t, expr)
			if n == nil {
				t.Fatal("frontend NFA")
			}
			d := buildDFAFromNFA(n)
			if d == nil {
				t.Fatal("small eligible DFA")
			}
			re := regexp.MustCompile(expr)
			inputs := [][]byte{nil, []byte("c"), []byte("b"), []byte("ac"), []byte("bd"), []byte("abac"), bytes.Repeat([]byte("a"), 64)}
			inputs = append(inputs, bytes.Repeat([]byte("a"), n.npos-1))
			rng := rand.New(rand.NewSource(10240))
			for j := 0; j < 256; j++ {
				data := make([]byte, rng.Intn(80))
				for i := range data {
					data[i] = []byte{'a', 'b', 'c', 'd', 0, ' '}[rng.Intn(6)]
				}
				inputs = append(inputs, data)
			}
			for _, data := range inputs {
				want := re.Match(data)
				if got := runDFAForTest(d, data); got != want || n.existsIn(data) != want {
					t.Fatalf("expr=%q data=%q DFA=%v NFA=%v oracle=%v", expr, data, got, n.existsIn(data), want)
				}
			}
		})
	}
}

func TestDFANativeCompileLoad(t *testing.T) {
	patterns := []Pattern{{ID: 0, Expr: `[ab][cd]`}, {ID: 1, Expr: `[ab]{2}`}}
	db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	image, err := db.MarshalImage()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadImage(image)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Close()
	for _, src := range []Database{db, loaded} {
		m := src.(*database).imageMVS()
		for idx, p := range patterns {
			if kind := m.kernel.dfaPlanKind(idx); kind != 1 {
				t.Fatalf("canonical native DFA not selected: %d", kind)
			}
			for _, data := range [][]byte{nil, []byte("c"), []byte("b"), []byte("ac"), []byte("aa"), {0xff, 'a', 'c'}, []byte("中ac")} {
				want := regexp.MustCompile(p.Expr).Match(data)
				if got := m.kernel.nfaExists(idx, data); got != want {
					t.Errorf("expr=%q data=%x native=%v oracle=%v", p.Expr, data, got, want)
				}
				// Ordinary Scan is a separate consumer using the merged NFA.
				if err := programCompare(programScan(t, src, data, 0), programOracle(patterns, data, BackendMVS, false, 0), false); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func TestProgramDFAFixture(t *testing.T) {
	dir := programEvidenceDir(t)
	db, err := Compile([]Pattern{{ID: 0, Expr: `[ab][cd]`}, {ID: 1, Expr: `[ab]{2}`}}, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	image, err := db.MarshalImage()
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{fmt.Sprintf("dfa-image-v%d.rehs", imageVersion): image, fmt.Sprintf("dfa-blob-v%d.bin", mvsBlobVersion): db.(*database).imageMVS().blob} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Log(fmt.Sprintf("image=%d blob=%d", len(image), len(db.(*database).imageMVS().blob)))
}

func TestDFAEligibility(t *testing.T) {
	for _, expr := range []string{`^ab`, `ab$`, strings.Repeat("a", 65)} {
		n := buildNFAFor(t, expr)
		if n == nil {
			t.Fatal(expr)
		}
		if buildDFAFromNFA(n) != nil {
			t.Fatalf("unsupported plan has DFA: %q", expr)
		}
	}
	n, ok := buildAssertNFA(t, `\b[ab]{2}\b`)
	if !ok {
		t.Fatal("assert frontend")
	}
	if buildDFAFromNFA(n) != nil {
		t.Fatal("DFA must not erase assertion semantics")
	}
}

func TestDFAStateBudget(t *testing.T) {
	small := buildNFAFor(t, `[ab]*a[ab]{6}`)
	if d := buildDFAFromNFA(small); d == nil || d.nstates >= 256 {
		t.Fatal("expected a DFA within the state budget")
	}
	large := buildNFAFor(t, `[ab]*a[ab]{8}`)
	if buildDFAFromNFA(large) != nil {
		t.Fatal("state explosion must decline to original NFA")
	}
	patterns := []Pattern{{ID: 0, Expr: `[ab]*a[ab]{8}`}}
	db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	image, err := db.MarshalImage()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadImage(image)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Close()
	for _, src := range []Database{db, loaded} {
		for _, data := range [][]byte{[]byte("abbbbbbbb"), []byte("abbbbbbb"), []byte("bbbbbbbbbb"), []byte("xx abbbbbbbb ")} {
			if err := programCompare(programScan(t, src, data, 0), programOracle(patterns, data, BackendMVS, false, 0), false); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func checkLegacyDFAConsumers(t *testing.T, k *mvsKernel) {
	t.Helper()
	exprs := []string{`[ab][cd]`, `[ab]{2}`}
	for _, data := range [][]byte{nil, []byte("c"), []byte("b"), []byte("ac"), []byte("aa"), []byte("aa ac"), {0xff, 'a', 'c'}, []byte("中aa")} {
		var wantBatch []int32
		many := k.nfaExistsMany([]int32{0, 1}, data, &scratch{})
		for idx, expr := range exprs {
			want := regexp.MustCompile(expr).Match(data)
			if k.dfaPlanKind(idx) != 2 {
				t.Fatal("legacy DFA must be marked for NFA fallback")
			}
			if k.nfaExists(idx, data) != want || k.nfaExistsScalar(idx, data) != want || len(many) != 2 || int8(many[idx]) < 0 || (many[idx] != 0) != want {
				t.Fatalf("legacy consumer idx=%d data=%x many=%v want=%v", idx, data, many, want)
			}
			if want {
				wantBatch = append(wantBatch, int32(idx))
			}
		}
		if got := k.dfaScanBatch([]int32{0, 1}, data, &scratch{}); !reflect.DeepEqual(got, wantBatch) && !(len(got) == 0 && len(wantBatch) == 0) {
			t.Fatalf("DFA batch: data=%x got=%v want=%v", data, got, wantBatch)
		}
	}
}

func TestDFALegacyImageAndBlob(t *testing.T) {
	image, err := os.ReadFile("testdata/compile-image-v5-dfa.rehs")
	if err != nil {
		t.Fatal(err)
	}
	blob, err := os.ReadFile("testdata/native-blob-v3-dfa.bin")
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(image[4:]) != 5 || binary.LittleEndian.Uint32(blob[4:]) != 3 {
		t.Fatal("fixture identity")
	}
	parses, attempts := 0, 0
	imageParseHook = func() { parses++ }
	imageNFAHook = func() { attempts++ }
	defer func() { imageParseHook = nil; imageNFAHook = nil }()
	db, err := LoadImage(image)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if parses != 0 || attempts != 0 {
		t.Fatal("legacy load invoked frontend")
	}
	again, err := db.MarshalImage()
	if err != nil || !bytes.Equal(again, image) {
		t.Fatal("legacy byte-identical export", err)
	}
	checkLegacyDFAConsumers(t, db.(*database).imageMVS().kernel)
	k := openMVSKernel(blob, 2)
	if k == nil {
		t.Fatal("legacy standalone native open")
	}
	defer k.close()
	checkLegacyDFAConsumers(t, k)
	packed, err := packImageBlob(blob, 2)
	if err != nil || string(packed[:4]) != "MVP1" {
		t.Fatal("legacy packed marker", err)
	}
	raw, err := unpackImageBlob(packed, 2)
	if err != nil || !bytes.Equal(raw, blob) {
		t.Fatal("legacy blob silently relabeled", err)
	}
	// Deliberate dense/packed reserialization also retains the old blob's
	// semantic marker, although public MarshalImage returns the original bytes.
	for _, packed := range []bool{false, true} {
		encoded, err := db.(*database).marshalCompilationImage(packed)
		if err != nil {
			t.Fatal(err)
		}
		if binary.LittleEndian.Uint32(encoded[8:]) != 3 {
			t.Fatal("changed blob semantics")
		}
		reloaded, err := LoadImage(encoded)
		if err != nil {
			t.Fatal(err)
		}
		checkLegacyDFAConsumers(t, reloaded.(*database).imageMVS().kernel)
		reloaded.Close()
		binary.LittleEndian.PutUint32(encoded[8:], 4)
		imageFixCRC(encoded)
		if bad, err := LoadImage(encoded); !errors.Is(err, ErrCorruptImage) {
			if bad != nil {
				bad.Close()
			}
			t.Fatal("header/body version mismatch accepted", err)
		}
	}
}

func TestDFAOldCacheRefreshOnce(t *testing.T) {
	old, err := os.ReadFile("testdata/compile-image-v5-dfa.rehs")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cache.rehs")
	if err := os.WriteFile(path, old, 0600); err != nil {
		t.Fatal(err)
	}
	compiles := 0
	imageCompileHook = func() { compiles++ }
	defer func() { imageCompileHook = nil }()
	for i := 0; i < 2; i++ {
		db, err := LoadOrCompile(path, []Pattern{{ID: 0, Expr: `[ab][cd]`}, {ID: 1, Expr: `[ab]{2}`}}, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatal(err)
		}
		m := db.(*database).imageMVS()
		if m.kernel.dfaPlanKind(0) != 1 || m.kernel.nfaExists(0, []byte("c")) {
			db.Close()
			t.Fatal("cache retained wrong DFA")
		}
		db.Close()
		if compiles != 1 {
			t.Fatalf("iteration=%d compiles=%d", i, compiles)
		}
	}
	current, err := os.ReadFile(path)
	if err != nil || binary.LittleEndian.Uint32(current[4:]) != imageVersion || binary.LittleEndian.Uint32(current[8:]) != mvsBlobVersion {
		t.Fatal("cache versions", err)
	}
}
