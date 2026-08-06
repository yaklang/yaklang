package lowhttp

import (
	"strings"
	"testing"
)

func FuzzHTTPHeaderFoldHelpersMatchLegacy(f *testing.F) {
	for _, seed := range []struct {
		value string
	}{
		{value: "Content-Length"},
		{value: "MULTIPART/FORM-DATA; boundary=abc"},
		{value: "gzip, CHUNKED"},
		{value: "chunKed"},
		{value: "invalid-\xff"},
		{value: ""},
	} {
		f.Add(seed.value)
	}

	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 64*1024 {
			t.Skip()
		}
		lower := strings.ToLower(value)
		for _, target := range []string{"content-length", "content-type", "transfer-encoding"} {
			if got, want := equalASCIIFoldOrLower(value, target), lower == target; got != want {
				t.Fatalf("equal %q with %q: got %v want %v", value, target, got, want)
			}
		}
		for _, prefix := range []string{"multipart/form-data", "chunked", ""} {
			if got, want := hasPrefixASCIIFoldOrLower(value, prefix), strings.HasPrefix(lower, prefix); got != want {
				t.Fatalf("prefix %q with %q: got %v want %v", value, prefix, got, want)
			}
		}
		for _, needle := range []string{"chunked", "multipart/form-data", ""} {
			if got, want := containsASCIIFoldOrLower(value, needle), strings.Contains(lower, needle); got != want {
				t.Fatalf("contains %q with %q: got %v want %v", value, needle, got, want)
			}
		}
	})
}

var benchmarkHTTPHeaderClassification bool

func BenchmarkHTTPHeaderClassification(b *testing.B) {
	headers := []string{
		"HoSt: example.test",
		"UsEr-AgEnT: yakit-performance",
		"AcCePt: text/html,application/json",
		"AcCePt-EnCoDiNg: gzip, deflate",
		"CaChE-CoNtRoL: no-cache",
		"PrAgMa: no-cache",
		"CoNnEcTiOn: keep-alive",
		"X-ReQuEsT-Id: 1234567890",
		"X-FoRwArDeD-FoR: 127.0.0.1",
		"CoNtEnT-TyPe: application/json",
		"CoNtEnT-LeNgTh: 4096",
		"TrAnSfEr-EnCoDiNg: gzip, chunked",
	}

	b.Run("legacy-lowercase", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var haveContentLength, isMultipart, haveChunked bool
			for _, line := range headers {
				key, value := SplitHTTPHeader(line)
				keyLower := strings.ToLower(key)
				valueLower := strings.ToLower(value)
				if !haveContentLength && keyLower == "content-length" {
					haveContentLength = true
				}
				if !isMultipart && keyLower == "content-type" && strings.HasPrefix(valueLower, "multipart/form-data") {
					isMultipart = true
				}
				if !haveChunked && keyLower == "transfer-encoding" && strings.Contains(valueLower, "chunked") {
					haveChunked = true
				}
			}
			benchmarkHTTPHeaderClassification = haveContentLength && haveChunked && !isMultipart
		}
	})

	b.Run("ascii-fold", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var haveContentLength, isMultipart, haveChunked bool
			for _, line := range headers {
				key, value := SplitHTTPHeader(line)
				if !haveContentLength && equalASCIIFoldOrLower(key, "content-length") {
					haveContentLength = true
				}
				if !isMultipart &&
					equalASCIIFoldOrLower(key, "content-type") &&
					hasPrefixASCIIFoldOrLower(value, "multipart/form-data") {
					isMultipart = true
				}
				if !haveChunked &&
					equalASCIIFoldOrLower(key, "transfer-encoding") &&
					containsASCIIFoldOrLower(value, "chunked") {
					haveChunked = true
				}
			}
			benchmarkHTTPHeaderClassification = haveContentLength && haveChunked && !isMultipart
		}
	})
}
