package minirehs

import (
	"bytes"
	"reflect"
	"sort"
	"testing"
)

func TestSharedVerifierSubjectScans(t *testing.T) {
	pats := []Pattern{
		{ID: 1, Expr: `prefix\Kpayload`},
		{ID: 2, Expr: `(?|(a)|(b))\1`},
		{ID: 3, Expr: `(?<word>你好)\k<word>`},
		{ID: 4, Expr: `token=(?=secret)secret`},
	}
	for _, backend := range []BackendKind{BackendMVS, BackendEngine, BackendStdlib} {
		t.Run(backend.String(), func(t *testing.T) {
			db, err := Compile(pats, WithBackend(backend), WithReportLocation(false), WithLogger(silentLogger{}))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			s, err := db.NewScratch()
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			sc := s.(*scratch)
			// Reuse the same backing bytes across different records, including
			// large asynchronous records and early handler termination.
			buffer := make([]byte, 0, 64<<10)
			for _, input := range [][]byte{
				bytes.Repeat([]byte("prefixpayload token=secret bb 你好你好 "), 1000),
				[]byte("unrelated"), nil, {0x80}, []byte("prefixpayload"),
				[]byte("你好你好 token=secret"), []byte("unrelated again"),
			} {
				buffer = append(buffer[:0], input...)
				var want []PatternID
				for _, p := range pats {
					// This oracle keeps the original per-verifier string conversion.
					v := &pcre2Verifier{expr: p.Expr}
					if len(v.findAll(buffer)) != 0 {
						want = append(want, p.ID)
					}
				}
				var got []PatternID
				if err := db.Scan(buffer, sc, func(m Match) bool {
					got = append(got, m.ID)
					return true
				}); err != nil {
					t.Fatal(err)
				}
				sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("bytes=%d got=%v want=%v", len(buffer), got, want)
				}
				if sc.verifierText != "" || sc.gateAsyncScratch != nil && sc.gateAsyncScratch.verifierText != "" {
					t.Fatal("scan retained its subject copy")
				}
				if err := db.Scan(buffer, sc, func(Match) bool { return false }); err != nil {
					t.Fatal(err)
				}
				if sc.verifierText != "" || sc.gateAsyncScratch != nil && sc.gateAsyncScratch.verifierText != "" {
					t.Fatal("early stop retained its subject copy")
				}
			}
		})
	}
}

func TestSharedVerifierSubjectWindows(t *testing.T) {
	for _, expr := range []string{`(?<=prefix)payload`, `\Apayload`, `payload\z`, `(?<word>你好)\k<word>`} {
		v := &pcre2Verifier{expr: expr}
		data := []byte("prefixpayload 你好你好")
		sc := &scratch{}
		for _, window := range [][2]int{{6, 13}, {14, 17}, {14, len(data)}, {6, len(data)}, {0, len(data)}, {0, 0}} {
			lo, hi := window[0], window[1]
			want := v.findAll(data[lo:hi])
			got := findAllInRecord(v, data, lo, hi, sc)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("expr=%q window=%v got=%v want=%v", expr, window, got, want)
			}
		}
	}
}
