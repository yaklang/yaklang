package jsonrecord

import (
	"context"
	"strings"
	"testing"
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
func FuzzParse(f *testing.F) {
	f.Add([]byte(`{"name":"a","version":"1"}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > MaxBytes {
			return
		}
		Parse(context.Background(), b)
	})
}
