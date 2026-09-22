package jsonrecord

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
)

func TestLocationsAndEscapedKeys(t *testing.T) {
	n, e := Parse(context.Background(), []byte("{\n \"a\\u002fb\": {\n \"n\": 9999999999999999999999999\n }\n}"))
	if e != nil {
		t.Fatal(e)
	}
	a, b := n.Get("a/b").Lines()
	if a != 2 || b != 4 {
		t.Fatalf("lines %d %d", a, b)
	}
	if string(n.Get("a/b", "n").Raw) != "9999999999999999999999999" {
		t.Fatal("numeric precision lost")
	}
}
func TestRejectAmbiguousAndBounded(t *testing.T) {
	for _, s := range []string{`{"a":1,"\u0061":2}`, `{} {}`, `[1,]`, strings.Repeat("[", 66) + strings.Repeat("]", 66), "\xff"} {
		if _, e := Parse(context.Background(), []byte(s)); e == nil {
			t.Fatalf("accepted %q", s)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := Parse(ctx, []byte(`{}`)); e == nil {
		t.Fatal("ignored cancellation")
	}
}
func TestDecodeChargesBeforeUnmarshal(t *testing.T) {
	raw := []byte(`{"name":"a","version":"1"}`)
	l, err := (budget.Limits{MaxResultBytes: 5000}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	ctx := budget.Bind(context.Background(), l)
	var rec struct{ Name, Version string }
	_, err = Decode(ctx, raw, &rec)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("second JSON mapping must be charged before Unmarshal: rec=%+v err=%v", rec, err)
	}
	if rec.Name != "" {
		t.Fatal("Unmarshal ran after budget exhaustion")
	}
	ctx = budget.Bind(context.Background(), budget.Limits{})
	n, err := Decode(ctx, raw, &rec)
	if err != nil || n == nil || rec.Name != "a" {
		t.Fatalf("positive decode: %+v %v", rec, err)
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(`{"name":"a","version":"1"}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > MaxBytes {
			return
		}
		Parse(context.Background(), b)
	})
}

func TestFieldLimitBeforeDecoderToken(t *testing.T) {
	for _, input := range []string{`"` + strings.Repeat("x", 64) + `"`, `{"` + strings.Repeat("x", 64) + `":1}`, `{} "` + strings.Repeat("x", 64) + `"`, strings.Repeat("9", 64)} {
		ctx := budget.Bind(context.Background(), budget.Limits{MaxFieldBytes: 32})
		p := &reader{ctx: ctx, raw: []byte(input), dec: json.NewDecoder(strings.NewReader(input)), limits: budget.From(ctx).Limits}
		if strings.HasPrefix(input, "{") {
			if _, err := p.token(); err != nil {
				t.Fatal(err)
			}
		}
		if strings.HasPrefix(input, "{}") {
			if _, err := p.token(); err != nil {
				t.Fatal(err)
			}
		}
		before := p.dec.InputOffset()
		_, err := p.token()
		if !errors.Is(err, scanerr.ErrResourceLimit) || p.dec.InputOffset() != before {
			t.Fatalf("token consumed before field limit: offset %d -> %d, err %v", before, p.dec.InputOffset(), err)
		}
	}
	n, err := Parse(budget.Bind(context.Background(), budget.Limits{MaxFieldBytes: 32}), []byte(`{"small":"value"}`))
	if err != nil || string(n.Get("small").Raw) != `"value"` {
		t.Fatalf("positive: %+v %v", n, err)
	}
}
