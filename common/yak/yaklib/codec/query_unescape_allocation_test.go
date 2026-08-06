package codec

import (
	"net/url"
	"testing"
)

func TestForceQueryUnescapeFastPathMatchesLegacy(t *testing.T) {
	inputs := []string{
		"", "plain", "already decoded 中文", "a/b?c=d&e", "a+b", "a%20b",
		"%u4f60%u597d", "%U4F60", "%zz", "%", "+", "100%25",
		string([]byte{'a', 0, 0xff, 'b'}),
	}
	for _, input := range inputs {
		want, err := url.QueryUnescape(UrlUnicodeDecode(input))
		if err != nil {
			want = input
		}
		if got := ForceQueryUnescape(input); got != want {
			t.Fatalf("ForceQueryUnescape(%q) = %q, want %q", input, got, want)
		}
	}
}
