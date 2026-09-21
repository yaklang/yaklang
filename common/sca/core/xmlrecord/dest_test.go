package xmlrecord

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
)

func TestCountStartsPropertiesNamespace(t *testing.T) {
	l, err := (budget.Limits{}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	count := func(raw string) int64 {
		t.Helper()
		ctx := budget.Bind(context.Background(), l)
		_, props, err := countStarts(ctx, decoderWithCharset([]byte(raw), false), l)
		if err != nil {
			t.Fatal(err)
		}
		return props
	}
	none := `<project><properties><foo>a</foo><bar>b</bar></properties></project>`
	maven := `<project xmlns="http://maven.apache.org/POM/4.0.0"><properties><foo>a</foo><bar>b</bar></properties></project>`
	if n := count(none); n != 2 {
		t.Fatalf("no-xmlns props=%d", n)
	}
	if n := count(maven); n != 2 {
		t.Fatalf("default Maven xmlns properties.UnmarshalXML keys Local, but prescan props=%d", n)
	}
}

func TestAnonymousPointerEmbedInspect(t *testing.T) {
	type item struct {
		Padding [65536]byte `xml:"-"`
		Value   string      `xml:",chardata"`
	}
	type doc struct {
		*item
	}
	var out doc
	_, err := inspectDest(&out)
	if err == nil || !strings.Contains(err.Error(), "unsupported_syntax") {
		t.Fatalf("anonymous *struct must not be treated as charged inline: %v", err)
	}
}

func TestXMLNameAndPathTagStayMapped(t *testing.T) {
	type rec struct {
		XMLName xml.Name `xml:"root"`
		Inner   string   `xml:"a>b"`
	}
	var out rec
	if err := decodeTest(context.Background(), strings.NewReader(`<root><a><b>x</b></a></root>`), &out); err != nil || out.Inner != "x" {
		t.Fatalf("path tag/XMLName: %+v %v", out, err)
	}
	if _, err := inspectDest(&out); err != nil {
		t.Fatalf("inspectDest of XMLName+path DTO: %v", err)
	}
}

func TestPrescanStackAndKeyBeforeAlloc(t *testing.T) {
	type rec struct {
		Items []string `xml:"item"`
	}
	shallowWide := func(n int) string {
		var b strings.Builder
		b.WriteString("<root>")
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "<a%d/>", i)
		}
		b.WriteString("</root>")
		return b.String()
	}
	deepNarrow := func(n int) string {
		return strings.Repeat("<d>", n) + "x" + strings.Repeat("</d>", n)
	}
	repeated := func(n int) string {
		return "<root>" + strings.Repeat("<item>x</item>", n) + "</root>"
	}
	high, err := (budget.Limits{MaxResultBytes: 8 << 20}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	prescan := func(raw string) int64 {
		t.Helper()
		ctx := budget.Bind(context.Background(), high)
		_, _, err := countStarts(ctx, decoderWithCharset([]byte(raw), false), high)
		if err != nil {
			t.Fatal(err)
		}
		return budget.From(ctx).ResultBytes()
	}
	// Repeat lookups must not Insert a new key per occurrence.
	r8, r40 := prescan(repeated(8)), prescan(repeated(40))
	u8, u40 := prescan(shallowWide(8)), prescan(shallowWide(40))
	d8, d24 := prescan(deepNarrow(8)), prescan(deepNarrow(24))
	t.Logf("prescan repeat8=%d repeat40=%d unique8=%d unique40=%d deep8=%d deep24=%d", r8, r40, u8, u40, d8, d24)
	if (u40-u8)*2 < (r40 - r8) {
		t.Fatalf("repeat path lookups charged like unique keys: unique Δ=%d repeat Δ=%d", u40-u8, r40-r8)
	}
	if d24 <= d8 {
		t.Fatalf("deep stack/path copies must grow: 8=%d 24=%d", d8, d24)
	}

	decode := func(raw string, max int64, stale rec) (rec, error) {
		t.Helper()
		l, err := (budget.Limits{MaxResultBytes: max}).Normalize()
		if err != nil {
			t.Fatal(err)
		}
		out := stale
		err = decodeTest(budget.Bind(context.Background(), l), strings.NewReader(raw), &out)
		return out, err
	}
	// Small documents still map.
	if out, err := decode(repeated(2), 8000, rec{}); err != nil || len(out.Items) != 2 || out.Items[0] != "x" {
		t.Fatalf("small repeat: %+v %v", out, err)
	}
	if out, err := decode(shallowWide(2), 8000, rec{}); err != nil || len(out.Items) != 0 {
		t.Fatalf("small unique unmatched tags: %+v %v", out, err)
	}
	if out, err := decode(deepNarrow(4), 8000, rec{}); err != nil {
		t.Fatalf("small deep: %+v %v", out, err)
	}

	// Unique-path growth refuses before filling a stale destination.
	stale := rec{Items: []string{"stale"}}
	out, err := decode(shallowWide(80), 8000, stale)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("unique 80 paths: err=%v out=%+v", err, out)
	}
	if len(out.Items) != 1 || out.Items[0] != "stale" {
		t.Fatalf("unique-path refusal mutated out: %+v", out)
	}
	out, err = decode(deepNarrow(40), 8000, stale)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) && !strings.Contains(fmt.Sprint(err), "resource_limit") {
		t.Fatalf("deep 40: err=%v out=%+v", err, out)
	}
	if len(out.Items) != 1 || out.Items[0] != "stale" {
		t.Fatalf("deep refusal mutated out: %+v", out)
	}

	// Same-path repeats under the unique-path refusal budget still map; the
	// lookup scratch is reserved once, not Inserted per item.
	out, err = decode(repeated(8), 8000, rec{})
	if err != nil || len(out.Items) != 8 {
		t.Fatalf("repeat 8 under unique-path budget: n=%d err=%v", len(out.Items), err)
	}
	stale = rec{Items: []string{"stale"}}
	out, err = decode(repeated(80), 8000, stale)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("repeat dest 80: err=%v n=%d", err, len(out.Items))
	}
	if len(out.Items) != 1 || out.Items[0] != "stale" {
		t.Fatalf("repeat dest refusal mutated out: %+v", out)
	}
}

func TestPathKeyScratchReuse(t *testing.T) {
	stack := []xml.Name{{Local: "root"}, {Local: "item", Space: "urn:demo"}}
	need := int(pathKeySize(stack))
	if need != 19 {
		t.Fatalf("frozen 19-byte path: %d", need)
	}
	// Cumulative AllocsPerRun of N rewrites, not live heap or RSS.
	for _, n := range []int{10, 100, 1000} {
		buf := make([]byte, 0, need)
		allocs := testing.AllocsPerRun(10, func() {
			for i := 0; i < n; i++ {
				buf = writePathScratch(buf, stack)
			}
		})
		t.Logf("identical_path_%d_rewrites allocations=%.0f cap=%d bytes=%d (cumulative, not live)", n, allocs, cap(buf), need)
		if allocs != 0 {
			t.Fatalf("reusable scratch %d rewrites allocated %.0f", n, allocs)
		}
		if cap(buf) < need || len(buf) != need {
			t.Fatalf("scratch cap=%d len=%d want >=%d", cap(buf), len(buf), need)
		}
	}
	buf := make([]byte, 0, 64)
	buf = writePathScratch(buf, stack)
	stored := string(buf)
	buf = writePathScratch(buf, []xml.Name{{Local: "other"}})
	if stored == string(buf) {
		t.Fatal("scratch overwrite did not change live buffer")
	}
	if stored != string(writePathScratch(nil, stack)) {
		t.Fatalf("cloned map key mutated with scratch reuse: %q", stored)
	}
}
