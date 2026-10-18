//go:build cgo

package minirehs

import (
	"bytes"
	"reflect"
	"testing"
)

// Compare only the confirmation algorithm with identical rules, native NFA,
// scratch and record in the same binary, excluding compilation and lazy startup.
func BenchmarkTeddyGetRequestConfirmation(b *testing.B) {
	pats := getRequestImagePatterns(b)
	fast, data := compileImageTest(b, pats, false)
	legacy, err := LoadImage(data)
	if err != nil {
		b.Fatal(err)
	}
	defer legacy.Close()
	m := legacy.(*database).imageMVS()
	m.pf.(prefilterReleaser).release()
	pf := newCGOPrefilterWithIndex(&literalIndex{literals: m.literals, litToPat: m.litToPat}, false)
	if pf == nil || pf.indexedConfirm() {
		b.Fatal("legacy confirmation constructor failed")
	}
	m.pf = pf
	record := bytes.Repeat([]byte("HTTP/1.1 200 OK\r\nServer: nginx/1.24.0\r\nContent-Type: text/html\r\n\r\n<html> token=abc123 header-042: abcd;ef </html>\n"), 8)
	want, e := imageMatches(fast, record)
	got, e2 := imageMatches(legacy, record)
	if e != nil || e2 != nil || !reflect.DeepEqual(want, got) {
		b.Fatal("legacy/indexed hits differ")
	}
	for _, pair := range []struct {
		name string
		db   Database
	}{{"Indexed", fast}, {"OriginalBuckets", legacy}} {
		b.Run(pair.name, func(b *testing.B) {
			sc, err := pair.db.NewScratch()
			if err != nil {
				b.Fatal(err)
			}
			defer sc.Close()
			_ = pair.db.Scan(record, sc, nil)
			b.SetBytes(int64(len(record)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err = pair.db.Scan(record, sc, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
