package rpm

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
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
				if !reflect.DeepEqual(oraclePackage(got[i]), oraclePackage(want[i])) {
					t.Fatalf("record %d:\ngot %+v\nwant %+v", i, oraclePackage(got[i]), oraclePackage(want[i]))
				}
			}
		})
	}
}
func oraclePackage(p *PackageInfo) PackageInfo {
	if p == nil {
		return PackageInfo{}
	}
	return PackageInfo{Name: p.Name, Version: p.Version, Release: p.Release, Arch: p.Arch, License: p.License, SigMD5: p.SigMD5, Epoch: p.Epoch, Provides: p.Provides, Requires: p.Requires}
}

func TestReadBudgetsAndCancel(t *testing.T) {
	b := make([]byte, 100)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := Parse(ctx, bytes.NewReader(b), 100, Limits{}); e == nil || !errors.Is(e, context.Canceled) && !errors.Is(e, scanerr.ErrCancelled) {
		t.Fatalf("ignored cancel: %v", e)
	}
	if _, e := Parse(context.Background(), bytes.NewReader(b), 100, Limits{MaxReadBytes: 50}); e == nil || !errors.Is(e, scanerr.ErrResourceLimit) {
		t.Fatalf("ignored budget: %v", e)
	}
}
func TestMalformedHeader(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"nil", nil},
		{"short", make([]byte, 8)},
		{"negative il", []byte{0xe3, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30}},
		{"all-ff", []byte{255, 255, 255, 255, 255, 255, 255, 255}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := header(tc.data); err == nil {
				t.Fatal("accepted malformed header")
			}
		})
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
				q.ProvideDeps = nil
				q.RequireDeps = nil
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

type hdrTag struct {
	tag, typ uint32
	strs     []string
	u32s     []uint32
}

func packHeader(tags []hdrTag) []byte {
	var data []byte
	var entries []byte
	for _, t := range tags {
		off := uint32(len(data))
		typ, count := t.typ, uint32(0)
		if t.strs != nil {
			if typ == 0 {
				if len(t.strs) == 1 {
					typ = 6
				} else {
					typ = 8
				}
			}
			count = uint32(len(t.strs))
			for _, s := range t.strs {
				data = append(data, s...)
				data = append(data, 0)
			}
		} else if t.u32s != nil {
			typ = 4
			count = uint32(len(t.u32s))
			for _, v := range t.u32s {
				var b [4]byte
				be.PutUint32(b[:], v)
				data = append(data, b[:]...)
			}
		}
		e := make([]byte, 16)
		be.PutUint32(e, t.tag)
		be.PutUint32(e[4:], typ)
		be.PutUint32(e[8:], off)
		be.PutUint32(e[12:], count)
		entries = append(entries, e...)
	}
	out := make([]byte, 8+len(entries)+len(data))
	be.PutUint32(out, uint32(len(tags)))
	be.PutUint32(out[4:], uint32(len(data)))
	copy(out[8:], entries)
	copy(out[8+len(entries):], data)
	return out
}

func identityTags(name, version string) []hdrTag {
	return []hdrTag{{tag: 1000, strs: []string{name}}, {tag: 1001, strs: []string{version}}}
}

func TestHeaderRequireVersionFlags(t *testing.T) {
	const equalConfig = uint32(268435464)
	b := packHeader(append(identityTags("mariner-release", "2.0"),
		hdrTag{tag: 1049, typ: 8, strs: []string{"config(mariner-release)", "rpmlib(CompressedFileNames)"}},
		hdrTag{tag: 1050, typ: 8, strs: []string{"2.0-4.cm2", "4.6.0-1"}},
		hdrTag{tag: 1048, u32s: []uint32{equalConfig, rpmSenseEqual | (1 << 24)}},
		hdrTag{tag: 1047, typ: 8, strs: []string{"config(mariner-release)", "mariner-release"}},
		hdrTag{tag: 1113, typ: 8, strs: []string{"2.0-4.cm2", "2.0"}},
		hdrTag{tag: 1112, u32s: []uint32{equalConfig, rpmSenseEqual}},
	))
	p, err := header(b)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "mariner-release" || len(p.Requires) != 2 || p.Requires[0] != "config(mariner-release)" {
		t.Fatalf("name oracle lost: %+v", p)
	}
	if len(p.RequireDeps) != 2 {
		t.Fatalf("require deps: %+v", p.RequireDeps)
	}
	d := p.RequireDeps[0]
	if d.Name != "config(mariner-release)" || d.Version != "2.0-4.cm2" || d.Flags != equalConfig {
		t.Fatalf("aligned require: %+v", d)
	}
	if d.Constraint() != "= 2.0-4.cm2" {
		t.Fatalf("constraint: %q", d.Constraint())
	}
	if p.ProvideDeps[0].Constraint() != "= 2.0-4.cm2" {
		t.Fatalf("provide constraint: %+v", p.ProvideDeps[0])
	}
}

func TestHeaderUnconstrainedRequire(t *testing.T) {
	b := packHeader(append(identityTags("x", "1"),
		hdrTag{tag: 1049, typ: 8, strs: []string{"/bin/sh"}},
		hdrTag{tag: 1050, typ: 8, strs: []string{""}},
		hdrTag{tag: 1048, u32s: []uint32{0}},
	))
	p, err := header(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.RequireDeps) != 1 || p.RequireDeps[0].Constraint() != "" || p.RequireDeps[0].Name != "/bin/sh" {
		t.Fatalf("unconstrained: %+v", p.RequireDeps)
	}
}

func TestHeaderDependencyArrayMismatch(t *testing.T) {
	for _, tc := range []struct {
		name string
		tags []hdrTag
	}{
		{"version-length", append(identityTags("x", "1"), hdrTag{tag: 1049, typ: 8, strs: []string{"a", "b"}}, hdrTag{tag: 1050, typ: 8, strs: []string{"1"}})},
		{"flag-length", append(identityTags("x", "1"), hdrTag{tag: 1049, typ: 8, strs: []string{"a"}}, hdrTag{tag: 1048, u32s: []uint32{8, 8}})},
		{"flags-without-names", append(identityTags("x", "1"), hdrTag{tag: 1048, u32s: []uint32{8}})},
		{"compare-without-version", append(identityTags("x", "1"), hdrTag{tag: 1049, typ: 8, strs: []string{"a"}}, hdrTag{tag: 1050, typ: 8, strs: []string{""}}, hdrTag{tag: 1048, u32s: []uint32{rpmSenseEqual}})},
		{"wrong-flag-type", append(identityTags("x", "1"), hdrTag{tag: 1048, typ: 8, strs: []string{"nope"}})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := header(packHeader(tc.tags))
			if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) && scanerr.CodeOf(err) != scanerr.MalformedInput {
				t.Fatalf("want malformed, got %v", err)
			}
			if errors.Is(err, scanerr.ErrResourceLimit) {
				t.Fatalf("mismatch classified as resource_limit: %v", err)
			}
		})
	}
}

func TestFrozenMarinerRequireConstraint(t *testing.T) {
	raw, err := os.ReadFile("testdata/cbl-mariner-2.0.gz")
	if err != nil {
		t.Fatal(err)
	}
	z, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(io.LimitReader(z, 64<<20))
	z.Close()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(context.Background(), bytes.NewReader(data), int64(len(data)), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	var pkg *PackageInfo
	for _, p := range got {
		if p.Name == "mariner-release" {
			pkg = p
			break
		}
	}
	if pkg == nil {
		t.Fatal("missing mariner-release")
	}
	found := false
	for _, d := range pkg.RequireDeps {
		if d.Name == "config(mariner-release)" {
			found = true
			if d.Version != "2.0-4.cm2" || d.Flags != 268435464 || d.Constraint() != "= 2.0-4.cm2" {
				t.Fatalf("frozen require: %+v", d)
			}
		}
	}
	if !found {
		t.Fatalf("config(mariner-release) missing: %+v", pkg.RequireDeps)
	}
}

func TestHeaderTypedResourceLimit(t *testing.T) {
	b := packHeader(append(identityTags("x", "1"), hdrTag{tag: 1049, typ: 8, strs: []string{"a", "b"}}))
	l, err := (budget.Limits{MaxResolveSteps: 1}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	_, err = headerWithContext(context.Background(), b, l)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("header traversal: %v", err)
	}
}
