package xmlrecord

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
)

func TestBoundedXML(t *testing.T) {
	var out struct {
		Value string `xml:"value"`
	}
	if err := decodeTest(context.Background(), strings.NewReader(`<root><value>works</value></root>`), &out); err != nil || out.Value != "works" {
		t.Fatalf("%+v %v", out, err)
	}
	for _, s := range []string{`<a/><b/>`, `<!DOCTYPE root SYSTEM "https://example.invalid/secret"><root/>`, strings.Repeat("<a>", 70) + strings.Repeat("</a>", 70)} {
		if err := decodeTest(context.Background(), strings.NewReader(s), &out); err == nil {
			t.Fatalf("accepted %q", s)
		}
	}
}

func TestDecodeChargesBeforeMapping(t *testing.T) {
	raw := `<root>` + strings.Repeat(`<value>xxxxxxxx</value>`, 80) + `</root>`
	l, err := (budget.Limits{MaxResultBytes: 8000}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Value []string `xml:"value"`
	}
	err = decodeTest(budget.Bind(context.Background(), l), strings.NewReader(raw), &out)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("XML destination mapping must be charged: out=%d err=%v", len(out.Value), err)
	}
	if len(out.Value) != 0 {
		t.Fatalf("Decode filled caller record after budget exhaustion: %d", len(out.Value))
	}
	out.Value = nil
	if err = decodeTest(context.Background(), strings.NewReader(`<root><value>works</value></root>`), &out); err != nil || len(out.Value) != 1 || out.Value[0] != "works" {
		t.Fatalf("positive: %+v %v", out, err)
	}
}

func TestDecodeDestinationBudgetSmallMediumLarge(t *testing.T) {
	type rec struct {
		Value []string `xml:"value"`
	}
	mk := func(n int) string {
		return `<root>` + strings.Repeat(`<value>xxxxxxxx</value>`, n) + `</root>`
	}
	// Small document under a low destination budget still maps.
	small := mk(2)
	l, err := (budget.Limits{MaxResultBytes: 8000}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	var out rec
	if err = decodeTest(budget.Bind(context.Background(), l), strings.NewReader(small), &out); err != nil || len(out.Value) != 2 {
		t.Fatalf("small: n=%d err=%v", len(out.Value), err)
	}
	// Medium: working copy fits 8000, destination object charge must refuse before filling out.
	medium := mk(80)
	if int64(len(medium)) >= 8000 {
		t.Fatalf("medium fixture must fit the working-copy cap, got %d", len(medium))
	}
	out.Value = []string{"stale"}
	err = decodeTest(budget.Bind(context.Background(), l), strings.NewReader(medium), &out)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("medium destination must be charged: out=%d err=%v", len(out.Value), err)
	}
	if len(out.Value) != 1 || out.Value[0] != "stale" {
		t.Fatalf("medium filled destination after refusal: %+v", out.Value)
	}
	// Large: working-copy cap refuses before Decode, destination stays untouched.
	large := mk(2000)
	out.Value = []string{"stale"}
	err = decodeTest(budget.Bind(context.Background(), l), strings.NewReader(large), &out)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("large working copy: out=%d err=%v", len(out.Value), err)
	}
	if len(out.Value) != 1 || out.Value[0] != "stale" {
		t.Fatalf("large filled destination: %+v", out.Value)
	}
	// Default budget still accepts the medium document.
	out.Value = nil
	if err = decodeTest(context.Background(), strings.NewReader(medium), &out); err != nil || len(out.Value) != 80 {
		t.Fatalf("medium default budget: n=%d err=%v", len(out.Value), err)
	}
}

func TestDeclaredEncodings(t *testing.T) {
	type record struct {
		Value string `xml:"value"`
	}
	latin := []byte("<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><r><value>caf\xe9</value></r>")
	var v record
	if err := decodeTest(context.Background(), bytes.NewReader(latin), &v); err != nil || v.Value != "café" {
		t.Fatalf("latin1: %+v %v", v, err)
	}
	text := "<?xml version=\"1.0\" encoding=\"UTF-16\"?><r><value>ok</value></r>"
	raw := []byte{0xff, 0xfe}
	for _, b := range []byte(text) {
		raw = append(raw, b, 0)
	}
	if err := decodeTest(context.Background(), bytes.NewReader(raw), &v); err != nil || v.Value != "ok" {
		t.Fatalf("utf16: %+v %v", v, err)
	}
}

func TestDecodeRejectsPaddingArrayDestination(t *testing.T) {
	type item struct {
		Padding [65536]byte `xml:"-"`
		Value   string      `xml:",chardata"`
	}
	type doc struct {
		Items []item `xml:"item"`
	}
	raw := "<root>" + strings.Repeat("<item/>", 8) + "</root>"
	l, err := (budget.Limits{MaxResultBytes: 8000}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	var d doc
	err = decodeTest(budget.Bind(context.Background(), l), strings.NewReader(raw), &d)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("64KiB dest elements must exceed 8000: items=%d charged=%d err=%v", len(d.Items), budget.From(budget.Bind(context.Background(), l)).ResultBytes(), err)
	}
	if len(d.Items) != 0 {
		t.Fatalf("filled destination before charge: %d", len(d.Items))
	}
}

func TestDecodeRejectsUnknownUnmarshaler(t *testing.T) {
	type stealth struct {
		N int `xml:"n"`
	}
	var s stealth
	err := decodeTest(context.Background(), strings.NewReader(`<root><n>1</n></root>`), &s)
	if err != nil {
		t.Fatalf("plain struct: %v", err)
	}
	var u hiddenUnmarshaler
	err = decodeTest(context.Background(), strings.NewReader(`<root><n>1</n></root>`), &u)
	if err == nil || !strings.Contains(err.Error(), "unsupported_syntax") {
		t.Fatalf("unknown Unmarshaler: %v", err)
	}
	if u.N != 0 {
		t.Fatalf("Unmarshaler ran: %+v", u)
	}
}

type hiddenUnmarshaler struct {
	N int `xml:"n"`
}

func (h *hiddenUnmarshaler) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	h.N = 99
	return d.Skip()
}

func TestDecodeSliceGrowthAndNested(t *testing.T) {
	type dep struct {
		Group    string `xml:"groupId"`
		Artifact string `xml:"artifactId"`
		Version  string `xml:"version"`
	}
	type project struct {
		XMLName struct{} `xml:"project"`
		Deps    []dep    `xml:"dependencies>dependency"`
	}
	body := func(n int) string {
		return `<project><dependencies>` + strings.Repeat(`<dependency><groupId>g</groupId><artifactId>a</artifactId><version>1</version></dependency>`, n) + `</dependencies></project>`
	}
	var out project
	if err := decodeTest(context.Background(), strings.NewReader(body(3)), &out); err != nil || len(out.Deps) != 3 {
		t.Fatalf("small nested: n=%d err=%v", len(out.Deps), err)
	}
	l, err := (budget.Limits{MaxResultBytes: 8000}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	out.Deps = []dep{{Group: "stale"}}
	err = decodeTest(budget.Bind(context.Background(), l), strings.NewReader(body(80)), &out)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("repeated dest must be charged: n=%d err=%v", len(out.Deps), err)
	}
	if len(out.Deps) != 1 || out.Deps[0].Group != "stale" {
		t.Fatalf("filled dest after refusal: %+v", out.Deps)
	}
	out.Deps = nil
	medium := body(8)
	if err := decodeTest(context.Background(), strings.NewReader(medium), &out); err != nil || len(out.Deps) != 8 {
		t.Fatalf("medium default: n=%d err=%v", len(out.Deps), err)
	}
}

func TestXMLDestinationSubprocess(t *testing.T) {
	if os.Getenv("XMLRECORD_DEST_CHILD") == "1" {
		type item struct {
			Padding [65536]byte `xml:"-"`
			Value   string      `xml:",chardata"`
		}
		type doc struct {
			Items []item `xml:"item"`
		}
		run := func(limit int64, n int) (int, error) {
			l, err := (budget.Limits{MaxResultBytes: limit}).Normalize()
			if err != nil {
				return -1, err
			}
			var d doc
			raw := "<root>" + strings.Repeat("<item/>", n) + "</root>"
			err = decodeTest(budget.Bind(context.Background(), l), strings.NewReader(raw), &d)
			return len(d.Items), err
		}
		if items, err := run(8000, 8); err == nil || items != 0 {
			t.Fatalf("low budget child items=%d err=%v", items, err)
		}
		if items, err := run(2<<20, 8); err != nil || items != 8 {
			t.Fatalf("2MiB child items=%d err=%v", items, err)
		}
		if items, err := run(2<<20, 16); err != nil || items != 16 {
			t.Fatalf("double growth child items=%d err=%v", items, err)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestXMLDestinationSubprocess$", "-test.v")
	cmd.Env = append(os.Environ(), "XMLRECORD_DEST_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child %v\n%s", err, out)
	}
}

func TestDecodePointerPointeeBudget(t *testing.T) {
	type item struct {
		Padding [65536]byte `xml:"-"`
		Value   string      `xml:",chardata"`
	}
	type doc struct {
		Items *item `xml:"item"`
	}
	raw := "<root><item/></root>"
	l, err := (budget.Limits{MaxResultBytes: 8000}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	stale := &item{Value: "stale"}
	var d doc
	d.Items = stale
	ctx := budget.Bind(context.Background(), l)
	err = decodeTest(ctx, strings.NewReader(raw), &d)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("pointer pointee must be charged: filled=%v charged=%d err=%v", d.Items != nil && d.Items != stale, budget.From(ctx).ResultBytes(), err)
	}
	if d.Items != stale || d.Items.Value != "stale" {
		t.Fatalf("low budget changed out: %+v", d.Items)
	}
	var ok doc
	if err := decodeTest(context.Background(), strings.NewReader(raw), &ok); err != nil || ok.Items == nil {
		t.Fatalf("high budget pointer: %+v %v", ok.Items, err)
	}
}

func TestDecodePointerSliceAndNested(t *testing.T) {
	type item struct {
		Padding [65536]byte `xml:"-"`
		Value   string      `xml:",chardata"`
	}
	type inner struct {
		Child *item `xml:"item"`
	}
	type doc struct {
		Inner *inner  `xml:"inner"`
		Items []*item `xml:"item"`
	}
	raw := "<root><inner><item/></inner><item/></root>"
	l, err := (budget.Limits{MaxResultBytes: 8000}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	stale := &item{Value: "stale"}
	out := doc{Items: []*item{stale}}
	err = decodeTest(budget.Bind(context.Background(), l), strings.NewReader(raw), &out)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("nested/slice pointers: inner=%v items=%d err=%v", out.Inner != nil, len(out.Items), err)
	}
	if len(out.Items) != 1 || out.Items[0] != stale || out.Inner != nil {
		t.Fatalf("low budget changed out: %+v", out)
	}
	var ok doc
	if err := decodeTest(context.Background(), strings.NewReader(raw), &ok); err != nil || ok.Inner == nil || ok.Inner.Child == nil || len(ok.Items) != 1 {
		t.Fatalf("high budget nested: %+v %v", ok, err)
	}
}

func TestDecodeRootSlice(t *testing.T) {
	type rec struct {
		XMLName xml.Name `xml:"value"`
		Body    string   `xml:",chardata"`
	}
	raw := `<value>works</value>`
	var out []rec
	if err := decodeTest(context.Background(), strings.NewReader(raw), &out); err != nil || len(out) != 1 || out[0].Body != "works" {
		t.Fatalf("root slice: %+v %v", out, err)
	}
}

func TestDecodeRejectsAnonymousPointerEmbed(t *testing.T) {
	type item struct {
		Padding [65536]byte `xml:"-"`
		Value   string      `xml:",chardata"`
	}
	type doc struct {
		*item
	}
	stale := &item{Value: "stale"}
	out := doc{item: stale}
	err := decodeTest(context.Background(), strings.NewReader(`<root>x</root>`), &out)
	if err == nil || !strings.Contains(err.Error(), "unsupported_syntax") {
		t.Fatalf("anonymous *struct embed must be rejected: err=%v", err)
	}
	if out.item != stale || out.item.Value != "stale" {
		t.Fatalf("reject allocated/changed pointee: %+v", out.item)
	}
}

func TestDecodeRejectsUnmarshalText(t *testing.T) {
	var u hiddenText
	err := decodeTest(context.Background(), strings.NewReader(`<root>x</root>`), &u)
	if err == nil || !strings.Contains(err.Error(), "unsupported_syntax") {
		t.Fatalf("UnmarshalText: %v", err)
	}
	if u.N != 0 {
		t.Fatalf("UnmarshalText ran: %+v", u)
	}
}

type hiddenText struct {
	N int `xml:",chardata"`
}

func (h *hiddenText) UnmarshalText(b []byte) error {
	h.N = 7
	return nil
}

func TestXMLSliceCapDoubling(t *testing.T) {
	if xmlSliceCap(1) != 1 || xmlSliceCap(2) != 2 || xmlSliceCap(3) != 4 || xmlSliceCap(5) != 8 {
		t.Fatalf("Grow(1) caps: 1=%d 2=%d 3=%d 5=%d", xmlSliceCap(1), xmlSliceCap(2), xmlSliceCap(3), xmlSliceCap(5))
	}
	if xmlSliceCap(256) != 256 {
		t.Fatalf("256: %d", xmlSliceCap(256))
	}
	if xmlSliceCap(257) <= 256 {
		t.Fatalf("1.25x after 256: %d", xmlSliceCap(257))
	}
}

func TestDecodeSliceDoublingPoints(t *testing.T) {
	type rec struct {
		Value []string `xml:"value"`
	}
	mk := func(n int) string {
		return `<root>` + strings.Repeat(`<value>x</value>`, n) + `</root>`
	}
	charge := func(n int) int64 {
		t.Helper()
		l, err := (budget.Limits{MaxResultBytes: 8 << 20}).Normalize()
		if err != nil {
			t.Fatal(err)
		}
		ctx := budget.Bind(context.Background(), l)
		var out rec
		if err := decodeTest(ctx, strings.NewReader(mk(n)), &out); err != nil || len(out.Value) != n {
			t.Fatalf("n=%d out=%d err=%v", n, len(out.Value), err)
		}
		return budget.From(ctx).ResultBytes()
	}
	c2, c3 := charge(2), charge(3)
	if c3 <= c2 {
		t.Fatalf("cap 2->4 must increase charge: 2=%d 3=%d", c2, c3)
	}
	c1 := charge(1)
	if c2 <= c1 {
		t.Fatalf("cap 1->2 must increase charge: 1=%d 2=%d", c1, c2)
	}
}

func TestDecodeRepeatedPointerPerParent(t *testing.T) {
	type item struct {
		Padding [65536]byte `xml:"-"`
		Value   string      `xml:",chardata"`
	}
	type holder struct {
		Child *item `xml:"item"`
	}
	type doc struct {
		Items []holder `xml:"holder"`
	}
	raw := "<root>" + strings.Repeat("<holder><item/></holder>", 8) + "</root>"
	low, err := (budget.Limits{MaxResultBytes: 80000}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	stale := []holder{{Child: &item{Value: "stale"}}}
	out := doc{Items: stale}
	ctx := budget.Bind(context.Background(), low)
	err = decodeTest(ctx, strings.NewReader(raw), &out)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("8 pointees must exceed 80000: filled=%v charged=%d err=%v", len(out.Items) == 8, budget.From(ctx).ResultBytes(), err)
	}
	if len(out.Items) != 1 || out.Items[0].Child == nil || out.Items[0].Child.Value != "stale" {
		t.Fatalf("low budget changed out: %+v", out.Items)
	}
	high, err := (budget.Limits{MaxResultBytes: 2 << 20}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	var ok doc
	if err := decodeTest(budget.Bind(context.Background(), high), strings.NewReader(raw), &ok); err != nil || len(ok.Items) != 8 {
		t.Fatalf("high budget 8 holders: n=%d err=%v", len(ok.Items), err)
	}
	for i, h := range ok.Items {
		if h.Child == nil {
			t.Fatalf("holder %d missing pointee", i)
		}
	}

	// One parent, repeated tags: encoding/xml overwrites a single pointer.
	// 80000 fits one 64KiB pointee but not eight; multiplying by tag count would reject.
	type one struct {
		Child *item `xml:"item"`
	}
	many := "<root>" + strings.Repeat("<item/>", 8) + "</root>"
	var single one
	if err := decodeTest(budget.Bind(context.Background(), low), strings.NewReader(many), &single); err != nil || single.Child == nil {
		t.Fatalf("single parent 8 tags must charge one pointee under 80000: %+v %v", single.Child, err)
	}
	nested := "<root>" + strings.Repeat("<group>"+strings.Repeat("<holder><item/></holder>", 2)+"</group>", 3) + "</root>"
	type group struct {
		Holders []holder `xml:"holder"`
	}
	type groups struct {
		Groups []group `xml:"group"`
	}
	var g groups
	if err := decodeTest(budget.Bind(context.Background(), high), strings.NewReader(nested), &g); err != nil || len(g.Groups) != 3 {
		t.Fatalf("nested groups: n=%d err=%v", len(g.Groups), err)
	}
	var kids int
	for _, gr := range g.Groups {
		if len(gr.Holders) != 2 {
			t.Fatalf("holders per group: %d", len(gr.Holders))
		}
		for _, h := range gr.Holders {
			if h.Child != nil {
				kids++
			}
		}
	}
	if kids != 6 {
		t.Fatalf("nested pointer instances: %d", kids)
	}
	staleG := groups{Groups: []group{{Holders: []holder{{Child: &item{Value: "stale"}}}}}}
	err = decodeTest(budget.Bind(context.Background(), low), strings.NewReader(nested), &staleG)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("6 nested pointees must exceed 80000: err=%v", err)
	}
	if len(staleG.Groups) != 1 || staleG.Groups[0].Holders[0].Child.Value != "stale" {
		t.Fatalf("nested low budget changed out")
	}

	tiny, err := (budget.Limits{MaxResultBytes: 8000}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	staleOne := one{Child: &item{Value: "stale"}}
	err = decodeTest(budget.Bind(context.Background(), tiny), strings.NewReader(many), &staleOne)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("one 64KiB pointee exceeds 8000: err=%v", err)
	}
	if staleOne.Child == nil || staleOne.Child.Value != "stale" {
		t.Fatalf("tiny budget changed out: %+v", staleOne.Child)
	}
}

func TestDecodeNamespacePointer(t *testing.T) {
	type item struct {
		Padding [65536]byte `xml:"-"`
		Value   string      `xml:",chardata"`
	}
	type doc struct {
		Items *item `xml:"urn:demo item"`
	}
	match := `<root><item xmlns="urn:demo"/></root>`
	low, err := (budget.Limits{MaxResultBytes: 8000}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	stale := &item{Value: "stale"}
	out := doc{Items: stale}
	err = decodeTest(budget.Bind(context.Background(), low), strings.NewReader(match), &out)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("namespaced pointee must be charged: filled=%v err=%v", out.Items != stale && out.Items != nil, err)
	}
	if out.Items != stale || out.Items.Value != "stale" {
		t.Fatalf("low budget changed namespaced out: %+v", out.Items)
	}
	var ok doc
	if err := decodeTest(context.Background(), strings.NewReader(match), &ok); err != nil || ok.Items == nil {
		t.Fatalf("high budget namespace match: %+v %v", ok.Items, err)
	}
	var miss doc
	if err := decodeTest(context.Background(), strings.NewReader(`<root><item/></root>`), &miss); err != nil {
		t.Fatalf("namespace mismatch should not error: %v", err)
	}
	if miss.Items != nil {
		t.Fatalf("namespace mismatch filled: %+v", miss.Items)
	}
}

func TestXMLPointerSubprocess(t *testing.T) {
	if os.Getenv("XMLRECORD_PTR_CHILD") == "1" {
		type item struct {
			Padding [65536]byte `xml:"-"`
			Value   string      `xml:",chardata"`
		}
		type doc struct {
			Items *item `xml:"item"`
		}
		raw := "<root><item/></root>"
		low, err := (budget.Limits{MaxResultBytes: 8000}).Normalize()
		if err != nil {
			t.Fatal(err)
		}
		stale := &item{Value: "stale"}
		d := doc{Items: stale}
		err = decodeTest(budget.Bind(context.Background(), low), strings.NewReader(raw), &d)
		if err == nil || d.Items != stale {
			t.Fatalf("child low: filled=%v err=%v", d.Items != stale, err)
		}
		high, err := (budget.Limits{MaxResultBytes: 2 << 20}).Normalize()
		if err != nil {
			t.Fatal(err)
		}
		var ok doc
		if err = decodeTest(budget.Bind(context.Background(), high), strings.NewReader(raw), &ok); err != nil || ok.Items == nil {
			t.Fatalf("child high: %+v %v", ok.Items, err)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestXMLPointerSubprocess$", "-test.v")
	cmd.Env = append(os.Environ(), "XMLRECORD_PTR_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child %v\n%s", err, out)
	}
}

func FuzzXML(f *testing.F) {
	for _, s := range []string{`<project><name>x</name></project>`, `<x a="1"/>`, `<!DOCTYPE x SYSTEM "file:///outside"><x/>`, `<x>`, strings.Repeat("<x>", 100)} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1<<18 {
			return
		}
		var out struct {
			Name string `xml:"name"`
		}
		decodeTest(context.Background(), strings.NewReader(s), &out)
	})
}
