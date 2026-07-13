package minirehs

import (
	"reflect"
	"testing"
)

type indexedMatch struct {
	record int
	match  Match
}

func TestScanBatchMatchesSequentialOrder(t *testing.T) {
	patterns := []Pattern{
		{ID: 1, Expr: `token=[a-z]+`},
		{ID: 2, Expr: `\b[0-9]{3}\b`},
		{ID: 3, Expr: `(?m)^HEAD:`},
		{ID: 4, Expr: `[^x]{2,5}`},
	}
	db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	records := [][]byte{
		[]byte("HEAD: token=alpha 123"),
		[]byte("nothing relevant"),
		[]byte("token=beta\nHEAD: 456"),
		[]byte{0xff, ' ', '1', '2', '3', ' '},
		[]byte("token=gamma"),
	}
	seqSc, _ := db.NewScratch()
	defer seqSc.Close()
	var want []indexedMatch
	for i, rec := range records {
		if err := db.Scan(rec, seqSc, func(m Match) bool {
			want = append(want, indexedMatch{record: i, match: m})
			return true
		}); err != nil {
			t.Fatal(err)
		}
	}

	batchSc, _ := db.NewScratch()
	defer batchSc.Close()
	for pass := 0; pass < 2; pass++ {
		var got []indexedMatch
		if err := db.ScanBatch(records, batchSc, func(record int, m Match) bool {
			got = append(got, indexedMatch{record: record, match: m})
			return true
		}); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("pass=%d batch mismatch\n got=%v\nwant=%v", pass, got, want)
		}
	}
}

func TestScanBatchEarlyStopAndNilScratch(t *testing.T) {
	db, err := Compile([]Pattern{{ID: 1, Expr: `a+`}}, WithBackend(BackendMVS))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	records := [][]byte{[]byte("a"), []byte("aa"), []byte("aaa"), []byte("aaaa")}
	var got []int
	if err := db.ScanBatch(records, nil, func(record int, _ Match) bool {
		got = append(got, record)
		return len(got) < 3
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []int{0, 1, 2}) {
		t.Fatalf("early-stop callbacks=%v", got)
	}
	if err := db.ScanBatch(nil, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestScanBatchMITMRealTraffic(t *testing.T) {
	patterns := re2OnlyMITMPatternsT(t)
	db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	records, _ := loadCorpus(t)
	if testing.Short() && len(records) > 200 {
		records = records[:200]
	}
	seqSc, _ := db.NewScratch()
	defer seqSc.Close()
	want := make([]indexedMatch, 0, len(records)*4)
	for i, rec := range records {
		if err := db.Scan(rec, seqSc, func(m Match) bool {
			want = append(want, indexedMatch{record: i, match: m})
			return true
		}); err != nil {
			t.Fatal(err)
		}
	}
	batchSc, _ := db.NewScratch()
	defer batchSc.Close()
	got := make([]indexedMatch, 0, len(want))
	if err := db.ScanBatch(records, batchSc, func(record int, m Match) bool {
		got = append(got, indexedMatch{record: record, match: m})
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("batch real-traffic mismatch: got=%d want=%d", len(got), len(want))
	}
	t.Logf("batch/sequential identical: records=%d matches=%d", len(records), len(want))
}
