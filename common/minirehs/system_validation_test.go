package minirehs

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
)

type systemOracleRule struct {
	id            PatternID
	re            *regexp.Regexp
	pcre          *regexp_utils.YakRegexpUtils
	fallbackExact bool
}

type systemOracle struct {
	rules   []systemOracleRule
	backend BackendKind
	locate  bool
}

// Compile original expressions once for the large independent reference run.
// No minirehs verifier, prefilter, NFA or executable regexp is reused. Only the
// existing exact-span fallback policy is supplied separately for each rule.
func newSystemOracle(patterns []Pattern, backend BackendKind, locate bool, fallback []bool) *systemOracle {
	o := &systemOracle{backend: backend, locate: locate}
	for i, p := range patterns {
		r := systemOracleRule{id: p.ID}
		if i < len(fallback) {
			r.fallbackExact = fallback[i]
		}
		expr := buildExprWithFlags(p)
		re, err := regexp.Compile(expr)
		if err == nil {
			if backend == BackendMVS {
				re.Longest()
			}
			r.re = re
		} else {
			r.pcre = regexp_utils.NewYakRegexpUtils(expr)
		}
		o.rules = append(o.rules, r)
	}
	return o
}

func (o *systemOracle) scan(data []byte) programObservation {
	var out programObservation
	var text string
	textReady := false
	for _, r := range o.rules {
		if r.pcre != nil {
			if !textReady {
				text, textReady = string(data), true
			}
			match, err := r.pcre.MatchString(text)
			if err != nil {
				out.Error = programError(err)
				return out
			}
			if match {
				out.Callbacks = append(out.Callbacks, Match{ID: r.id, From: -1, To: -1})
			}
			continue
		}
		if o.backend == BackendMVS && !o.locate && !r.fallbackExact {
			if r.re.Match(data) {
				out.Callbacks = append(out.Callbacks, Match{ID: r.id, From: -1, To: -1})
			}
			continue
		}
		for _, loc := range r.re.FindAllIndex(data, -1) {
			out.Callbacks = append(out.Callbacks, Match{ID: r.id, From: loc[0], To: loc[1]})
		}
	}
	out.Completed = true
	return out
}

func TestSystemCachedOracleMatchesOriginal(t *testing.T) {
	pats := []Pattern{{ID: 1, Expr: `a|aa`}, {ID: 2, Expr: `(?i)[kz]{2}`}, {ID: 3, Expr: `abc(?=!)`}, {ID: 4, Expr: `a*`}}
	fallback := []bool{false, false, false, true}
	for _, backend := range []BackendKind{BackendMVS, BackendEngine, BackendStdlib} {
		for _, locate := range []bool{false, true} {
			o := newSystemOracle(pats, backend, locate, fallback)
			for _, data := range [][]byte{nil, []byte("aaaa abc! Kz\x00\xff"), []byte("nothing")} {
				want := programObservation{Completed: true}
				for i, p := range pats {
					one := programOracle([]Pattern{p}, data, backend, locate, 0, fallback[i])
					want.Callbacks = append(want.Callbacks, one.Callbacks...)
					if one.Error != nil {
						want.Completed, want.Error = false, one.Error
						break
					}
				}
				if err := programCompare(o.scan(data), want, true); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	invalid := []Pattern{{ID: 17, Expr: `(`}}
	if err := programCompare(newSystemOracle(invalid, BackendMVS, false, nil).scan([]byte("x")), programOracle(invalid, []byte("x"), BackendMVS, false, 0), true); err != nil {
		t.Fatal(err)
	}
}

func systemScan(t *testing.T, db Database, sc Scratch, data []byte, stop int) programObservation {
	t.Helper()
	var out programObservation
	err := db.Scan(data, sc, func(m Match) bool {
		out.Callbacks = append(out.Callbacks, m)
		out.Stopped = stop > 0 && len(out.Callbacks) >= stop
		return !out.Stopped
	})
	out.Error, out.Completed = programError(err), err == nil && !out.Stopped
	return out
}

func systemPrefix(full programObservation, stop int) programObservation {
	if stop > 0 && len(full.Callbacks) >= stop {
		full.Callbacks = full.Callbacks[:stop]
		full.Stopped, full.Completed, full.Error = true, false, nil
	}
	return full
}

// An evidence run must fail on missing/truncated input rather than silently
// accepting a skip or the ordinary helper's default first80-record limit.
func systemFullCorpus(t *testing.T) ([][]byte, string) {
	t.Helper()
	raw, err := os.ReadFile("testdata/traffic_corpus.bin")
	if err != nil {
		t.Fatal(err)
	}
	var records [][]byte
	for off := 0; off < len(raw); {
		if len(raw)-off < 4 {
			t.Fatal("truncated corpus length")
		}
		n := uint64(binary.LittleEndian.Uint32(raw[off : off+4]))
		off += 4
		if n > uint64(len(raw)-off) {
			t.Fatal("truncated corpus record")
		}
		records = append(records, raw[off:off+int(n)])
		off += int(n)
	}
	if len(records) != 1332 {
		t.Fatalf("expected actual1332 corpus records, got%d", len(records))
	}
	return records, fmt.Sprintf("%x", sha256.Sum256(raw))
}

func TestProgramSystemCorpus(t *testing.T) {
	dir := programEvidenceDir(t)
	corpus, corpusSHA := systemFullCorpus(t)
	type input struct {
		Name string
		Data []byte
	}
	var inputs []input
	for i, data := range corpus {
		inputs = append(inputs, input{fmt.Sprintf("Corpus%04d", i), data})
	}
	fixed := programRuntimeInputs()
	for _, name := range []string{"HTTP912", "Sparse16K", "DenseHTTP16K", "Zero16K"} {
		inputs = append(inputs, input{name, fixed[name]})
	}
	inputs = append(inputs, input{"Empty", nil}, input{"Spaces32", bytes.Repeat([]byte{' '}, 32)}, input{"Spaces128", bytes.Repeat([]byte{' '}, 128)},
		input{"UnicodeNUL", []byte("HTTP/1.1 200 OK\r\nServer: Kſ中\x00\r\nabc! foo bar\n")},
		input{"InvalidUTF", append(append([]byte{0xff, 0x80, 0xc0, 0xaf, 0, 0xe2}, fixed["HTTP912"]...), 0xe2, 0x82)})
	for _, where := range []string{"Head", "Middle", "EOF"} {
		data := bytes.Repeat([]byte{' '}, 64<<10)
		off := 0
		if where == "Middle" {
			off = len(data) / 2
		} else if where == "EOF" {
			off = len(data) - len(fixed["HTTP912"])
		}
		copy(data[off:], fixed["HTTP912"])
		inputs = append(inputs, input{"Sparse64K" + where, data})
	}
	type row struct {
		Input, SHA256 string
		Bytes, Stop   int
		Compile, Load programObservation
		Expected      programObservation
	}
	comparisons, fullRows, stopRows := 0, 0, 0
	var smallMode, singletonMode string
	for _, set := range []string{"GetRequest", "MITM"} {
		pats := getRequestImagePatterns(t)
		if set == "MITM" {
			pats, _ = compilableMITMPatterns(t)
		}
		for _, locate := range []bool{false, true} {
			db, err := Compile(pats, WithBackend(BackendMVS), WithReportLocation(locate), WithLogger(silentLogger{}))
			if err != nil {
				t.Fatal(err)
			}
			smallMode = programSmallWorkerMode(t, db)
			singletonMode = programMergedSingletonMode(t, db)
			m := db.(*database).imageMVS()
			fallback := make([]bool, len(pats))
			for i, cp := range m.all {
				fallback[i] = m.nfas[i] == nil && cp.v.exact()
			}
			o := newSystemOracle(pats, BackendMVS, locate, fallback)
			image, err := db.MarshalImage()
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadImage(image)
			if err != nil {
				t.Fatal(err)
			}
			if programSmallWorkerMode(t, loaded) != smallMode || programMergedSingletonMode(t, loaded) != singletonMode {
				t.Fatal("Compile/Load runtime plans differ")
			}
			a, err := db.NewScratch()
			if err != nil {
				t.Fatal(err)
			}
			b, err := loaded.NewScratch()
			if err != nil {
				t.Fatal(err)
			}
			file, err := os.Create(filepath.Join(dir, fmt.Sprintf("system-%s-locate-%v.jsonl", set, locate)))
			if err != nil {
				t.Fatal(err)
			}
			enc := json.NewEncoder(file)
			check := func(x, y programObservation, ordered bool, name string) {
				comparisons++
				if err := programCompare(x, y, ordered); err != nil {
					t.Fatalf("%s locate%v %s: %v", set, locate, name, err)
				}
			}
			for _, in := range inputs {
				sha := fmt.Sprintf("%x", sha256.Sum256(in.Data))
				want := o.scan(in.Data)
				x, y := systemScan(t, db, a, in.Data, 0), systemScan(t, loaded, b, in.Data, 0)
				if err := enc.Encode(row{in.Name, sha, len(in.Data), 0, x, y, want}); err != nil {
					t.Fatal(err)
				}
				check(x, want, false, in.Name+" compiled/oracle")
				check(y, want, false, in.Name+" loaded/oracle")
				check(x, y, true, in.Name+" compile/load order")
				if want.Error != nil || x.Error != nil || y.Error != nil {
					t.Fatal("whole-subject oracle/resource errors require explicit resolution", set, in.Name)
				}
				fullRows++
				stops := []int{1}
				if len(x.Callbacks) > 1 {
					stops = append(stops, len(x.Callbacks)/2+1)
				}
				for _, stop := range stops {
					prefix := systemPrefix(x, stop)
					cx, cy := systemScan(t, db, a, in.Data, stop), systemScan(t, loaded, b, in.Data, stop)
					if err := enc.Encode(row{in.Name, sha, len(in.Data), stop, cx, cy, prefix}); err != nil {
						t.Fatal(err)
					}
					check(cx, prefix, true, in.Name+" compiled stop")
					check(cy, prefix, true, in.Name+" loaded stop")
					check(cx, cy, true, in.Name+" stop compile/load")
					stopRows++
				}
				if fmt.Sprintf("%x", sha256.Sum256(in.Data)) != sha {
					t.Fatal("scan mutated original input", in.Name)
				}
			}
			file.Close()
			a.Close()
			b.Close()
			loaded.Close()
			db.Close()
		}
	}
	writeProgramJSON(t, dir, "system-corpus-summary.json", map[string]interface{}{"corpus_records": len(corpus), "corpus_container_sha256": corpusSHA, "input_cases": len(inputs), "full_rows": fullRows, "stop_rows": stopRows, "complete_comparisons": comparisons, "P": runtime.GOMAXPROCS(0), "logical_cpus": runtime.NumCPU(), "small_worker_mode": smallMode, "singleton_mode": singletonMode, "scope": "native Windows run recorded externally; this test itself is portable. Complete original expressions, reused owned scratch, both reporting modes/CompileLoad, ordered first/middle stops, immutable input. Raw JSONL contains recoverable tuples/errors/completion; no digest-only comparison."})
}

// Unlike the historical Engine/Stdlib subset benchmark, each operation here
// scans all1332 original records against all87 compilable MITM expressions.
func BenchmarkProgramSystemCorpus(b *testing.B) {
	patterns, _ := compilableMITMPatterns(b)
	records, _ := loadCorpusB(b)
	if len(records) != 1332 {
		b.Fatal("complete original corpus required; set MINIREHS_FULL_CORPUS=1")
	}
	var total int64
	for _, record := range records {
		total += int64(len(record))
	}
	for _, locate := range []bool{false, true} {
		b.Run(map[bool]string{false: "Exist", true: "Locate"}[locate], func(b *testing.B) {
			db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(locate), WithLogger(silentLogger{}))
			if err != nil {
				b.Fatal(err)
			}
			defer db.Close()
			programSmallWorkerMode(b, db)
			programMergedSingletonMode(b, db)
			sc, err := db.NewScratch()
			if err != nil {
				b.Fatal(err)
			}
			defer sc.Close()
			for _, record := range records {
				if err := db.Scan(record, sc, nil); err != nil {
					b.Fatal(err)
				}
			}
			b.SetBytes(total)
			b.ReportAllocs()
			b.ResetTimer()
			b.ReportMetric(1332, "records/op")
			for i := 0; i < b.N; i++ {
				for _, record := range records {
					if err := db.Scan(record, sc, nil); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

func TestProgramSystemLargeRecords(t *testing.T) {
	dir := programEvidenceDir(t)
	literals := make([]Pattern, 64)
	for i := range literals {
		literals[i] = Pattern{ID: PatternID(200 + i), Expr: fmt.Sprintf("dense-token-%03d", i)}
	}
	mitm, _ := compilableMITMPatterns(t)
	large := 1 << 20
	late := append([]byte("\"https://example.com/foo\" "), bytes.Repeat([]byte{' '}, large)...)
	late = append(late, []byte(" 99")...)
	pc := append(bytes.Repeat([]byte{' '}, large), []byte(" xfoo abab foo! \x00K\xff")...)
	type pair struct {
		Name, SHA256   string
		Locate         bool
		Bytes, Stop    int
		Actual, Expect programObservation
	}
	var pairs []pair
	for _, tc := range []struct {
		name string
		pats []Pattern
		data []byte
	}{
		{"LiteralDense1MiB", literals, bytes.Repeat([]byte("dense-token-042 "), large/16)},
		{"SingletonEarlyAssertLate1MiB", []Pattern{mitm[27], {ID: 9999, Expr: `\b[0-9]{2}\b`}}, late},
		{"PCREContext1MiB", []Pattern{{ID: 1, Expr: `foo(?=!)`}, {ID: 2, Expr: `(?<=x)foo`}, {ID: 3, Expr: `(ab)\1`}}, pc},
		{"WideAndNullable", []Pattern{{ID: 1, Expr: `[ab]{129}`}, {ID: 2, Expr: `a*`}, {ID: 3, Expr: `[ab]{64,65}`}}, []byte(" " + string(bytes.Repeat([]byte{'a'}, 257)) + "\xff\x00")},
	} {
		for _, locate := range []bool{false, true} {
			db, err := Compile(tc.pats, WithBackend(BackendMVS), WithReportLocation(locate), WithLogger(silentLogger{}))
			if err != nil {
				t.Fatal(tc.name, err)
			}
			m := db.(*database).imageMVS()
			fallback := make([]bool, len(tc.pats))
			for i, cp := range m.all {
				fallback[i] = m.nfas[i] == nil && cp.v.exact()
			}
			o := newSystemOracle(tc.pats, BackendMVS, locate, fallback)
			want := o.scan(tc.data)
			image, err := db.MarshalImage()
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadImage(image)
			if err != nil {
				t.Fatal(err)
			}
			for _, current := range []Database{db, loaded} {
				full := programScan(t, current, tc.data, 0)
				pairs = append(pairs, pair{tc.name, fmt.Sprintf("%x", sha256.Sum256(tc.data)), locate, len(tc.data), 0, full, want})
				if err := programCompare(full, want, false); err != nil {
					t.Fatal(tc.name, err)
				}
				for _, stop := range []int{1, len(full.Callbacks)/2 + 1} {
					got, prefix := programScan(t, current, tc.data, stop), systemPrefix(full, stop)
					pairs = append(pairs, pair{tc.name, fmt.Sprintf("%x", sha256.Sum256(tc.data)), locate, len(tc.data), stop, got, prefix})
					if err := programCompare(got, prefix, true); err != nil {
						t.Fatal(tc.name, err)
					}
				}
			}
			loaded.Close()
			db.Close()
		}
	}
	writeProgramJSON(t, dir, "system-large-complete-results.json", pairs)
}

func TestProgramSystemImageSizes(t *testing.T) {
	dir := programEvidenceDir(t)
	var rows []map[string]interface{}
	for _, set := range []string{"GetRequest", "MITM"} {
		pats := getRequestImagePatterns(t)
		if set == "MITM" {
			pats, _ = compilableMITMPatterns(t)
		}
		db, err := Compile(pats, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatal(err)
		}
		m := db.(*database).imageMVS()
		image, err := db.MarshalImage()
		if err != nil {
			t.Fatal(err)
		}
		packed, err := db.(*database).marshalCompilationImage(true)
		if err != nil {
			t.Fatal(err)
		}
		dense, err := db.(*database).marshalUncompressedImage()
		if err != nil {
			t.Fatal(err)
		}
		h, err := readImageHeader(image)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, map[string]interface{}{"set": set, "patterns": len(pats), "image_version": h.version, "blob_version": binary.LittleEndian.Uint32(m.blob[4:]), "compressed": h.compressed, "packed": h.packed, "default_image_bytes": len(image), "packed_uncompressed_image_bytes": len(packed), "dense_uncompressed_image_bytes": len(dense), "dense_native_blob_bytes": len(m.blob), "image_sha256": fmt.Sprintf("%x", sha256.Sum256(image)), "note": "File encoding and materialized dense runtime blob are separate. Packed/dense includes pattern/literal metadata; no RSS claim or Go/C/RSS summation."})
		if err := os.WriteFile(filepath.Join(dir, set+"-current.rehs"), image, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, set+"-current.blob"), m.blob, 0600); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
	writeProgramJSON(t, dir, "system-image-sizes.json", rows)
}
