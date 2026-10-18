package minirehs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageVersionSemanticMatrix(t *testing.T) {
	type fixture struct {
		name string
		data []byte
	}
	var fixtures []fixture
	for _, name := range []string{"compile-image-v1.rehs", "compile-image-v2.rehs", "compile-image-v3.rehs", "compile-image-v4.rehs", "compile-image-v4-invalid-utf-exist.rehs", "compile-image-v4-invalid-utf-located.rehs", "compile-image-v5-dfa.rehs"} {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		fixtures = append(fixtures, fixture{name, data})
	}
	pats := []Pattern{{ID: 1, Expr: `[ab][cd]`}, {ID: 2, Expr: `\bfoo\b`}, {ID: 3, Expr: `foo(?=!)`}, {ID: 4, Expr: `k[ab]token`, Flags: FlagCaseless}, {ID: 5, Expr: `[ab]{65}`}, {ID: 6, Expr: `^中.*文$`}, {ID: 7, Expr: `\x{FFFD}foo(?=!)`}}
	for _, locate := range []bool{false, true} {
		db, err := Compile(pats, WithBackend(BackendMVS), WithReportLocation(locate), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatal(err)
		}
		for _, packed := range []bool{false, true} {
			raw, err := db.(*database).marshalCompilationImage(packed)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := compressCompilationImage(raw)
			if err != nil {
				t.Fatal(err)
			}
			fixtures = append(fixtures, fixture{fmt.Sprintf("v6/loc%v/packed%v/raw", locate, packed), bytes.Clone(raw)}, fixture{fmt.Sprintf("v6/loc%v/packed%v/compressed", locate, packed), bytes.Clone(encoded)})
		}
		db.Close()
	}
	type pair struct {
		Fixture            string
		Image, Blob        uint32
		Locate             bool
		Packed, Compressed bool
		Input              []byte
		Stop               int
		Actual, Expected   programObservation
		Ordered            bool
	}
	var pairs []pair
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			original := bytes.Clone(f.data)
			h, err := readImageHeader(original)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(f.name, "v6/") && (h.packed != strings.Contains(f.name, "packedtrue") || h.compressed != strings.HasSuffix(f.name, "/compressed")) {
				t.Fatal("current matrix did not reach the named wire/codec representation")
			}
			var db Database
			func() {
				compiles, parses, nfas := 0, 0, 0
				imageCompileHook = func() { compiles++ }
				imageParseHook = func() { parses++ }
				imageNFAHook = func() { nfas++ }
				defer func() { imageCompileHook, imageParseHook, imageNFAHook = nil, nil, nil }()
				var err error
				db, err = LoadImage(f.data)
				if err != nil {
					t.Fatal(err)
				}
				if compiles != 0 || parses != 0 || nfas != 0 {
					t.Fatal("LoadImage invoked compilation")
				}
			}()
			defer db.Close()
			clear(f.data)
			exported, err := db.MarshalImage()
			if err != nil || !bytes.Equal(exported, original) {
				t.Fatal("input alias or byte-identical re-export violation", err)
			}
			clear(exported)
			exported, err = db.MarshalImage()
			if err != nil || !bytes.Equal(exported, original) {
				t.Fatal("export alias violation", err)
			}
			m := db.(*database).imageMVS()
			for _, data := range [][]byte{nil, []byte("c b ac bd ab bb aa"), []byte("aa hello world abcd token=abc foo!"), []byte("Katoken ſfoo foo"), []byte("中文"), []byte(strings.Repeat("a", 129)), {0xff, 'f', 'o', 'o', '!', 0, 'b', 'a', 'r', 0xff}, {0xe2, 0x82, 'f', 'o', 'o', '!'}} {
				want := programObservation{Completed: true}
				for idx, cp := range m.all {
					// Expressions in images already include their original i/s/m
					// modifiers. Only fallback span policy comes from route metadata;
					// matcher, all offsets and multiplicity come from the oracle.
					p := Pattern{ID: cp.id, Expr: cp.expr}
					one := programOracle([]Pattern{p}, data, BackendMVS, m.reportLoc, 0, m.nfas[idx] == nil && cp.v.exact())
					want.Callbacks = append(want.Callbacks, one.Callbacks...)
					if one.Error != nil {
						want.Error, want.Completed = one.Error, false
						break
					}
				}
				full := programScan(t, db, data, 0)
				if err := programCompare(full, want, false); err != nil {
					t.Fatal(err)
				}
				pairs = append(pairs, pair{f.name, h.version, h.blobVersion, m.reportLoc, h.packed, h.compressed, data, 0, full, want, false})
				for _, stop := range []int{1, 2} {
					expected := full
					if len(full.Callbacks) >= stop {
						expected.Callbacks = full.Callbacks[:stop]
						expected.Stopped, expected.Completed, expected.Error = true, false, nil
					}
					got := programScan(t, db, data, stop)
					if err := programCompare(got, expected, true); err != nil {
						t.Fatal(err)
					}
					pairs = append(pairs, pair{f.name, h.version, h.blobVersion, m.reportLoc, h.packed, h.compressed, data, stop, got, expected, true})
				}
			}
		})
	}
	if dir := os.Getenv("MINIREHS_EVIDENCE_DIR"); dir != "" {
		writeProgramJSON(t, programEvidenceDir(t), "image-version-complete-results.json", pairs)
	}
}

func TestImageRollbackUsesCorrectCacheNamespace(t *testing.T) {
	stableRules := []Pattern{{ID: 73, Expr: `stable[ab]token`}}
	nextRules := []Pattern{{ID: 73, Expr: `next[ab]token`}}
	dir := t.TempDir()
	stablePath, nextPath := filepath.Join(dir, "stable-v6.rehs"), filepath.Join(dir, "candidate-v6.rehs")
	opts := []Option{WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{})}
	stable, err := LoadOrCompile(stablePath, stableRules, opts...)
	if err != nil {
		t.Fatal(err)
	}
	defer stable.Close()
	stableBytes, err := os.ReadFile(stablePath)
	if err != nil {
		t.Fatal(err)
	}
	next, err := LoadOrCompile(nextPath, nextRules, opts...)
	if err != nil {
		t.Fatal(err)
	}
	next.Close() // This owner has no readers in flight.
	bad, err := os.ReadFile(nextPath)
	if err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint32(bad[4:], imageVersion+1)
	imageFixCRC(bad)
	if err := os.WriteFile(nextPath, bad, 0600); err != nil {
		t.Fatal(err)
	}
	compiles := 0
	imageCompileHook = func() { compiles++ }
	defer func() { imageCompileHook = nil }()
	if d, err := LoadImage(bad); !errors.Is(err, ErrIncompatibleImage) {
		if d != nil {
			d.Close()
		}
		t.Fatal("candidate failure must be explicit", err)
	}
	// The application explicitly chooses a previous, correctness-fixed plan.
	// LoadImage itself never compiles or silently changes the requested rules.
	rolled, err := LoadImage(stableBytes)
	if err != nil {
		t.Fatal(err)
	}
	defer rolled.Close()
	for _, subject := range [][]byte{[]byte("stableatoken"), []byte("nextatoken"), {0xff, 's', 't', 'a', 'b', 'l', 'e', 'b', 't', 'o', 'k', 'e', 'n'}} {
		want := programOracle(stableRules, subject, BackendMVS, false, 0)
		if err := programCompare(programScan(t, rolled, subject, 0), want, false); err != nil {
			t.Fatal(err)
		}
	}
	again, err := os.ReadFile(stablePath)
	if err != nil || !bytes.Equal(again, stableBytes) || compiles != 0 {
		t.Fatal("candidate publication/rejection changed stable cache or compiled", err, compiles)
	}
}

func TestImageMandatoryFeaturesAndCacheRecovery(t *testing.T) {
	pats := []Pattern{{ID: 0, Expr: `[ab][cd]`}, {ID: 1, Expr: `\bfoo\b`}}
	_, raw := compileRawImageTest(t, pats, false)
	for _, change := range []struct {
		name string
		off  int
		val  uint32
		want error
	}{
		{"future-image", 4, imageVersion + 1, ErrIncompatibleImage},
		{"future-blob", 8, mvsBlobVersion + 1, ErrIncompatibleImage},
		{"mandatory-feature", 12, 1 << 31, ErrIncompatibleImage},
		{"count-overflow", 28, ^uint32(0), ErrCorruptImage},
	} {
		t.Run(change.name, func(t *testing.T) {
			bad := bytes.Clone(raw)
			binary.LittleEndian.PutUint32(bad[change.off:], change.val)
			imageFixCRC(bad)
			if db, err := LoadImage(bad); !errors.Is(err, change.want) {
				if db != nil {
					db.Close()
				}
				t.Fatalf("bad feature/count accepted: %v", err)
			}
			cache := filepath.Join(t.TempDir(), "current-v6.rehs")
			if err := os.WriteFile(cache, bad, 0600); err != nil {
				t.Fatal(err)
			}
			compiles := 0
			imageCompileHook = func() { compiles++ }
			defer func() { imageCompileHook = nil }()
			for j := 0; j < 2; j++ {
				db, err := LoadOrCompile(cache, pats, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
				if err != nil {
					t.Fatal(err)
				}
				want := programOracle(pats, []byte("c ac foo"), BackendMVS, false, 0)
				if err := programCompare(programScan(t, db, []byte("c ac foo"), 0), want, false); err != nil {
					t.Fatal(err)
				}
				db.Close()
			}
			if compiles != 1 {
				t.Fatal("invalid cache must refresh once, then hit", compiles)
			}
		})
	}
}

func TestImageCacheFullPatternIdentity(t *testing.T) {
	wanted := []Pattern{{ID: 31, Expr: `wanted[ab]token`}, {ID: 31, Expr: `second[ab]token`, Flags: FlagCaseless}}
	for _, change := range []string{"expression", "id", "flags", "order"} {
		t.Run(change, func(t *testing.T) {
			foreign := append([]Pattern(nil), wanted...)
			switch change {
			case "expression":
				foreign[0].Expr = `foreign[ab]token`
			case "id":
				foreign[0].ID++
			case "flags":
				// Same effective expression, different original bytes/flags.
				foreign[1].Expr = "(?i)" + foreign[1].Expr
				foreign[1].Flags = 0
			case "order":
				foreign[0], foreign[1] = foreign[1], foreign[0]
			}
			_, raw := compileRawImageTest(t, foreign, false)
			cfg := newDefaultConfig()
			cfg.backend = BackendMVS
			cfg.reportLocation = false
			// Deliberately equal fingerprints model a collision or forged
			// header. CRC is integrity checking, not authentication.
			binary.LittleEndian.PutUint64(raw[20:], imageIdentity(wanted, cfg, compileImageFlags(cfg)))
			imageFixCRC(raw)
			data, err := compressCompilationImage(raw)
			if err != nil {
				t.Fatal(err)
			}
			cache := filepath.Join(t.TempDir(), "collision.rehs")
			if err := os.WriteFile(cache, data, 0600); err != nil {
				t.Fatal(err)
			}
			compiles := 0
			imageCompileHook = func() { compiles++ }
			defer func() { imageCompileHook = nil }()
			for j := 0; j < 2; j++ {
				db, err := LoadOrCompile(cache, wanted, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
				if err != nil {
					t.Fatal(err)
				}
				for _, subject := range [][]byte{[]byte("wantedatoken SECONDAtoken"), []byte("foreignatoken"), nil} {
					want := programOracle(wanted, subject, BackendMVS, false, 0)
					if err := programCompare(programScan(t, db, subject, 0), want, false); err != nil {
						db.Close()
						t.Fatal(err)
					}
				}
				db.Close()
			}
			if compiles != 1 {
				t.Fatal("full pattern mismatch must refresh once, then hit", compiles)
			}
		})
	}
}

func TestImageCacheFlagIdentityHits(t *testing.T) {
	var pats []Pattern
	for flags := Flag(0); flags < 8; flags++ {
		pats = append(pats, Pattern{ID: PatternID(flags), Expr: `^(?:ab.foo|[ab]token)$`, Flags: flags})
	}
	pats = append(pats, Pattern{ID: 8, Expr: `(?i)ab[cd]`, Flags: FlagCaseless})
	for _, locate := range []bool{false, true} {
		cache := filepath.Join(t.TempDir(), "flags.rehs")
		compiles := 0
		imageCompileHook = func() { compiles++ }
		for j := 0; j < 2; j++ {
			db, err := LoadOrCompile(cache, pats, WithBackend(BackendMVS), WithReportLocation(locate), WithLogger(silentLogger{}))
			if err != nil {
				imageCompileHook = nil
				t.Fatal(err)
			}
			for _, subject := range [][]byte{[]byte("AB\nfoo"), []byte("x\nabtToken\nabtoken\n"), []byte("abctoken"), nil} {
				if err := programCompare(programScan(t, db, subject, 0), programOracle(pats, subject, BackendMVS, locate, 0), false); err != nil {
					db.Close()
					imageCompileHook = nil
					t.Fatal(err)
				}
			}
			db.Close()
		}
		imageCompileHook = nil
		if compiles != 1 {
			t.Fatal("valid full-key cache must stay a hit", compiles)
		}
	}
}
