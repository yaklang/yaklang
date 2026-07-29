package codec

import (
	"bytes"
	"testing"

	"github.com/yaklang/yaklang/common/mimetype/mimeutil/mimecharset"
	"golang.org/x/net/html/charset"
)

func legacyDetectedCharsetDecode(raw []byte) ([]byte, bool) {
	charsetLower := mimecharset.FromPlain(raw)
	enc, _ := charset.Lookup(charsetLower)
	if enc == nil {
		return raw, false
	}
	fixed, err := enc.NewDecoder().Bytes(raw)
	if err != nil {
		return raw, false
	}
	return fixed, true
}

func TestTryUTF8ConvertorDetectedUTF8MatchesLegacyWithoutCopy(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
	}{
		{name: "ascii-json", raw: []byte(`{"status":"ok","payload":"ascii"}`)},
		{name: "utf8-json", raw: []byte(`{"status":"正常","payload":"中文"}`)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			legacy, legacyOK := legacyDetectedCharsetDecode(test.raw)
			if !legacyOK {
				t.Fatal("legacy UTF-8 detection did not decode the fixture")
			}

			result := &MIMEResult{MIMEType: "application/json", IsText: true}
			got, gotOK := result.TryUTF8Convertor(test.raw)
			if gotOK != legacyOK || !bytes.Equal(got, legacy) {
				t.Fatalf("TryUTF8Convertor() = (%q, %v), legacy = (%q, %v)", got, gotOK, legacy, legacyOK)
			}
			if len(got) > 0 && &got[0] != &test.raw[0] {
				t.Fatal("validated UTF-8 identity conversion allocated a body copy")
			}
		})
	}
}

func TestTryUTF8ConvertorDetectedUTF8KeepsReplacementGuard(t *testing.T) {
	raw := []byte("prefix\ufffdsuffix")
	result := &MIMEResult{MIMEType: "text/plain", IsText: true}

	got, ok := result.TryUTF8Convertor(raw)
	if ok {
		t.Fatal("TryUTF8Convertor reported success for a body containing the replacement rune")
	}
	if !bytes.Equal(got, raw) || &got[0] != &raw[0] {
		t.Fatal("replacement-rune guard did not return the original input")
	}
}

func TestTryUTF8ConvertorExplicitUTF8SemanticsUnchanged(t *testing.T) {
	raw := []byte("already utf-8")
	result := &MIMEResult{MIMEType: "text/plain", IsText: true, Charset: "utf-8"}

	got, ok := result.TryUTF8Convertor(raw)
	if ok {
		t.Fatal("explicit UTF-8 charset unexpectedly changed the conversion signal")
	}
	if !bytes.Equal(got, raw) || &got[0] != &raw[0] {
		t.Fatal("explicit UTF-8 charset did not return the original input")
	}
}

var (
	benchmarkUTF8ConvertorBytes []byte
	benchmarkUTF8ConvertorOK    bool
)

func BenchmarkTryUTF8ConvertorDetectedUTF8_256K(b *testing.B) {
	raw := bytes.Repeat([]byte(`{"payload":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`), 262144/46+1)
	raw = raw[:262144]
	result := &MIMEResult{MIMEType: "application/json", IsText: true}

	b.Run("legacy-identity-decoder", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(raw)))
		for i := 0; i < b.N; i++ {
			benchmarkUTF8ConvertorBytes, benchmarkUTF8ConvertorOK = legacyDetectedCharsetDecode(raw)
		}
	})

	b.Run("validated-utf8-handoff", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(raw)))
		for i := 0; i < b.N; i++ {
			benchmarkUTF8ConvertorBytes, benchmarkUTF8ConvertorOK = result.TryUTF8Convertor(raw)
		}
	})
}
