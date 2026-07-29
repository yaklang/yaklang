package utils

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func getHeaderValueListLegacy(header http.Header, key string) []string {
	if header == nil {
		return nil
	}
	cKey := http.CanonicalHeaderKey(key)
	if key == cKey {
		if raw, ok := header[key]; ok {
			return raw
		}
		return []string{}
	}

	v1 := header[key]
	v2 := header[cKey]
	vals := make([]string, 0, len(v1)+len(v2))
	seen := map[string]any{}
	for _, values := range [][]string{v1, v2} {
		for _, value := range values {
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = value
			vals = append(vals, value)
		}
	}
	return vals
}

func TestGetHeaderValueListFastPathMatchesLegacy(t *testing.T) {
	for _, test := range []struct {
		name   string
		header http.Header
		key    string
	}{
		{
			name:   "canonical common",
			header: http.Header{"Content-Length": {"4096"}},
			key:    "Content-Length",
		},
		{
			name:   "lower common canonical storage",
			header: http.Header{"Content-Length": {"4096"}},
			key:    "content-length",
		},
		{
			name:   "lower common lower storage",
			header: http.Header{"content-length": {"4096"}},
			key:    "content-length",
		},
		{
			name: "mixed storage",
			header: http.Header{
				"x-custom": {"lower", "duplicate"},
				"X-Custom": {"canonical", "duplicate"},
			},
			key: "x-custom",
		},
		{
			name:   "empty values",
			header: http.Header{"Host": {"", "example.test", ""}},
			key:    "host",
		},
		{
			name:   "duplicate values",
			header: http.Header{"Transfer-Encoding": {"chunked", "chunked"}},
			key:    "transfer-encoding",
		},
		{
			name: "large unique fallback",
			header: http.Header{"X-Large": {
				"one", "two", "three", "four", "five",
				"six", "seven", "eight", "nine",
			}},
			key: "x-large",
		},
		{
			name:   "missing",
			header: make(http.Header),
			key:    "content-length",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			want := getHeaderValueListLegacy(test.header, test.key)
			got := getHeaderValueList(test.header, test.key)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %#v, want %#v", got, want)
			}
		})
	}
}

func FuzzGetHeaderValueListFastPathMatchesLegacy(f *testing.F) {
	f.Add("content-length", "4096", "", "4096", "")
	f.Add("x-custom", "one", "duplicate", "two", "duplicate")
	f.Add("host", "", "example.test", "", "")
	f.Fuzz(func(t *testing.T, key, v1a, v1b, v2a, v2b string) {
		if len(key)+len(v1a)+len(v1b)+len(v2a)+len(v2b) > 4096 {
			t.Skip()
		}
		key = strings.ReplaceAll(strings.ReplaceAll(key, "\r", ""), "\n", "")
		canonical := http.CanonicalHeaderKey(key)
		header := make(http.Header)
		header[key] = []string{v1a, v1b}
		if canonical != key {
			header[canonical] = []string{v2a, v2b}
		}

		want := getHeaderValueListLegacy(header, key)
		got := getHeaderValueList(header, key)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("key %q: got %#v, want %#v", key, got, want)
		}
	})
}

var benchmarkHeaderValues []string

func BenchmarkGetHeaderValueListFastPath(b *testing.B) {
	header := http.Header{
		"Content-Length":    {"4096"},
		"Host":              {"performance.example.test"},
		"Transfer-Encoding": {"chunked"},
	}
	keys := []string{"content-length", "host", "transfer-encoding"}
	for _, benchmark := range []struct {
		name string
		fn   func(http.Header, string) []string
	}{
		{name: "legacy", fn: getHeaderValueListLegacy},
		{name: "normalized_fast_path", fn: getHeaderValueList},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				benchmarkHeaderValues = benchmark.fn(header, keys[index%len(keys)])
			}
		})
	}
}
