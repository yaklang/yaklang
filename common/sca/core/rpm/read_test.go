package rpm

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"sort"
	"testing"
)

// Full outputs come from isolated go-rpmdb v0.1.0, never from this reader.
// rpm-qa.json additionally preserves upstream independent command expectations.
func TestUpstreamDatabaseMatrix(t *testing.T) {
	raw, err := os.ReadFile("testdata/expected.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string][]*PackageInfo
	if err = json.Unmarshal(raw, &expected); err != nil {
		t.Fatal(err)
	}
	for name, want := range expected {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile("testdata/" + name + ".gz")
			if err != nil {
				t.Fatal(err)
			}
			z, err := gzip.NewReader(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(io.LimitReader(z, 64<<20))
			if err != nil {
				t.Fatal(err)
			}
			z.Close()
			got, err := Parse(context.Background(), bytes.NewReader(data), int64(len(data)), Limits{})
			if err != nil {
				t.Fatal(err)
			}
			less := func(p []*PackageInfo) func(int, int) bool {
				return func(i, j int) bool { return p[i].Name+p[i].Version+p[i].Arch < p[j].Name+p[j].Version+p[j].Arch }
			}
			sort.Slice(got, less(got))
			sort.Slice(want, less(want))
			if len(got) != len(want) {
				t.Fatalf("records %d want %d", len(got), len(want))
			}
			for i := range want {
				if !reflect.DeepEqual(got[i], want[i]) {
					t.Fatalf("record %d:\ngot %+v\nwant %+v", i, got[i], want[i])
				}
			}
		})
	}
}
func TestReadBudgetsAndCancel(t *testing.T) {
	b := make([]byte, 100)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := Parse(ctx, bytes.NewReader(b), 100, Limits{}); e == nil {
		t.Fatal("ignored cancel")
	}
	if _, e := Parse(context.Background(), bytes.NewReader(b), 100, Limits{MaxReadBytes: 50}); e == nil {
		t.Fatal("ignored budget")
	}
}
func TestMalformedHeader(t *testing.T) {
	for _, b := range [][]byte{nil, make([]byte, 8), {255, 255, 255, 255, 255, 255, 255, 255}} {
		if _, err := header(b); err == nil {
			t.Fatal("accepted malformed header")
		}
	}
}
func FuzzDatabase(f *testing.F) {
	for _, name := range []string{"libuuid", "sle15-bci", "cbl-mariner-2.0"} {
		raw, err := os.ReadFile("testdata/" + name + ".gz")
		if err != nil {
			f.Fatal(err)
		}
		z, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			f.Fatal(err)
		}
		b, err := io.ReadAll(z)
		z.Close()
		if err != nil {
			f.Fatal(err)
		}
		if _, err = Parse(context.Background(), bytes.NewReader(b), int64(len(b)), Limits{}); err != nil {
			f.Fatal("invalid backend seed", err)
		}
		f.Add(b)
		f.Add(b[:len(b)/2])
	}

	f.Add([]byte("SQLite format 3\x00"))
	f.Add([]byte("RpmP"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 8<<20 {
			return
		}
		Parse(context.Background(), bytes.NewReader(b), int64(len(b)), Limits{MaxReadBytes: 32 << 20, MaxRecords: 2000, MaxRecordBytes: 4 << 20, MaxPageVisits: 10000})
	})
}

func TestIndependentRPMCommandExpectations(t *testing.T) {
	raw, err := os.ReadFile("testdata/rpm-qa.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string][]*PackageInfo
	if err = json.Unmarshal(raw, &expected); err != nil {
		t.Fatal(err)
	}
	for name, want := range expected {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile("testdata/" + name + ".gz")
			if err != nil {
				t.Fatal(err)
			}
			z, err := gzip.NewReader(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(io.LimitReader(z, 64<<20))
			z.Close()
			if err != nil {
				t.Fatal(err)
			}
			got, err := Parse(context.Background(), bytes.NewReader(b), int64(len(b)), Limits{})
			if err != nil {
				t.Fatal(err)
			}
			actual := map[string]bool{}
			for _, p := range got {
				q := *p
				q.Provides = nil
				q.Requires = nil
				v, _ := json.Marshal(q)
				actual[string(v)] = true
			}
			// The pinned upstream command list for centos6-many covers 44 of 326 records.
			// All other command lists cover the whole database.
			if name != "centos6-many" && len(got) != len(want) {
				t.Fatalf("count %d want %d", len(got), len(want))
			}
			for _, q := range want {
				v, _ := json.Marshal(q)
				if !actual[string(v)] {
					t.Fatalf("missing independent record %s", v)
				}
			}
		})
	}
}

func FuzzHeader(f *testing.F) {
	// Two big-endian RPM string tags, no database dependency.
	b := make([]byte, 44)
	be.PutUint32(b, 2)
	be.PutUint32(b[4:], 4)
	for i, tag := range []uint32{1000, 1001} {
		e := b[8+16*i:]
		be.PutUint32(e, tag)
		be.PutUint32(e[4:], 6)
		be.PutUint32(e[8:], uint32(i*2))
		be.PutUint32(e[12:], 1)
	}
	copy(b[40:], []byte{'x', 0, '1', 0})
	if p, e := header(b); e != nil || p.Name != "x" || p.Version != "1" {
		f.Fatal("invalid semantic seed", e)
	}
	f.Add(b)
	f.Add(b[:20])
	f.Add([]byte{255, 255, 255, 255, 255, 255, 255, 255})
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) <= 1<<20 {
			header(b)
		}
	})
}
