package xmlrecord

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestBoundedXML(t *testing.T) {
	var out struct {
		Value string `xml:"value"`
	}
	if err := Decode(context.Background(), strings.NewReader(`<root><value>works</value></root>`), &out); err != nil || out.Value != "works" {
		t.Fatalf("%+v %v", out, err)
	}
	for _, s := range []string{`<a/><b/>`, `<!DOCTYPE root SYSTEM "https://example.invalid/secret"><root/>`, strings.Repeat("<a>", 70) + strings.Repeat("</a>", 70)} {
		if err := Decode(context.Background(), strings.NewReader(s), &out); err == nil {
			t.Fatalf("accepted %q", s)
		}
	}
}

func TestDeclaredEncodings(t *testing.T) {
	type record struct {
		Value string `xml:"value"`
	}
	latin := []byte("<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><r><value>caf\xe9</value></r>")
	var v record
	if err := Decode(context.Background(), bytes.NewReader(latin), &v); err != nil || v.Value != "café" {
		t.Fatalf("latin1: %+v %v", v, err)
	}
	text := "<?xml version=\"1.0\" encoding=\"UTF-16\"?><r><value>ok</value></r>"
	raw := []byte{0xff, 0xfe}
	for _, b := range []byte(text) {
		raw = append(raw, b, 0)
	}
	if err := Decode(context.Background(), bytes.NewReader(raw), &v); err != nil || v.Value != "ok" {
		t.Fatalf("utf16: %+v %v", v, err)
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
		Decode(context.Background(), strings.NewReader(s), &out)
	})
}
