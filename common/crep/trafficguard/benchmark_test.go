package trafficguard

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// Benchmark 证明: 内置超级正则组一次编译、一次扫描, 纯净流量与命中流量都极快,
// 满足 MITM 实时热路径(目标: 普通 HTTP 事务扫描 P95 <= 2ms)。

func benchScanner() *Scanner {
	s, err := NewScanner()
	if err != nil {
		panic(err)
	}
	return s
}

func makeNoisyBody(n int) []byte {
	// 模拟真实 HTTP 响应: JSON / HTML 混合, 不含任何凭证(测纯净流量的"快速排除"能力)。
	chunk := `{"items":[{"id":1,"name":"widget","price":9.99},{"id":2,"name":"gadget","price":19.99}],` +
		`"meta":{"page":1,"total":42},"html":"<div class=\"card\">hello world</div>"}`
	b := make([]byte, 0, n+len(chunk))
	for len(b) < n {
		b = append(b, chunk...)
		b = append(b, '\n')
	}
	return b[:n]
}

func makeHitBody(baseSize int) []byte {
	// 模拟含一条 AWS AKIA + 一条 GitHub Token 的真实响应。
	b := makeNoisyBody(baseSize)
	tail := []byte(fmt.Sprintf(" akid=AKIAIOSFODNN7EXAMPLE repo=https://x token=ghp_%s",
		strings.Repeat("a", 36)))
	return append(b, tail...)
}

func makeRepeatedJSONBody(n int) []byte {
	prefix := []byte(`{"status":"ok","payload":"`)
	suffix := []byte(`"}`)
	if n < len(prefix)+len(suffix) {
		return []byte(strings.Repeat("a", n))
	}
	body := make([]byte, n)
	copy(body, prefix)
	for i := len(prefix); i < n-len(suffix); i++ {
		body[i] = 'a'
	}
	copy(body[n-len(suffix):], suffix)
	return body
}

func legacyDiscordTokenCandidateForBenchmark(data []byte) bool {
	const tokenLength = 24 + 1 + 6 + 1 + 27
	for start := 0; start+tokenLength <= len(data); start++ {
		if data[start] != 'M' && data[start] != 'N' {
			continue
		}
		valid := true
		for index := start + 1; index < start+24; index++ {
			if !isTokenAlphabet(data[index]) {
				valid = false
				break
			}
		}
		if !valid || data[start+24] != '.' {
			continue
		}
		for index := start + 25; index < start+31; index++ {
			if !isTokenAlphabet(data[index]) {
				valid = false
				break
			}
		}
		if !valid || data[start+31] != '.' {
			continue
		}
		for index := start + 32; index < start+tokenLength; index++ {
			if !isTokenAlphabet(data[index]) {
				valid = false
				break
			}
		}
		if valid {
			return true
		}
	}
	return false
}

func BenchmarkDiscordTokenCandidate256K(b *testing.B) {
	validToken := []byte("M" + strings.Repeat("a", 23) + "." + strings.Repeat("b", 6) + "." + strings.Repeat("c", 27))
	validAtEnd := makeRepeatedJSONBody(256 * 1024)
	copy(validAtEnd[len(validAtEnd)-len(validToken):], validToken)
	fixtures := map[string][]byte{
		"repeated-json": makeRepeatedJSONBody(256 * 1024),
		"natural-json":  makeNoisyBody(256 * 1024),
		"valid-at-end":  validAtEnd,
		"dense-dots":    bytes.Repeat([]byte("a."), 128*1024),
		"dense-prefix":  bytes.Repeat([]byte("MN"), 128*1024),
		"dense-both":    bytes.Repeat([]byte("MN."), 256*1024/3+1)[:256*1024],
	}
	implementations := []struct {
		name string
		fn   func([]byte) bool
	}{
		{name: "legacy", fn: legacyDiscordTokenCandidateForBenchmark},
		{name: "indexed", fn: hasDiscordTokenCandidate},
	}
	for fixtureName, data := range fixtures {
		data := data
		b.Run(fixtureName, func(b *testing.B) {
			for _, implementation := range implementations {
				implementation := implementation
				b.Run(implementation.name, func(b *testing.B) {
					b.SetBytes(int64(len(data)))
					for i := 0; i < b.N; i++ {
						_ = implementation.fn(data)
					}
				})
			}
		})
	}
}

func BenchmarkScanClean32K(b *testing.B) {
	s := benchScanner()
	data := makeNoisyBody(32 * 1024)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.ScanRequest(data)
	}
}

func BenchmarkScanClean256K(b *testing.B) {
	s := benchScanner()
	data := makeNoisyBody(256 * 1024)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.ScanRequest(data)
	}
}

// BenchmarkScanCleanRepeated256K mirrors the Electron MITM performance
// fixture. The long low-entropy payload is deliberately kept separate from
// makeNoisyBody because SIMD fingerprint filters can behave very differently
// on repetitive and natural-language inputs.
func BenchmarkScanCleanRepeated256K(b *testing.B) {
	s := benchScanner()
	data := makeRepeatedJSONBody(256 * 1024)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.ScanResponse(data)
	}
}

func BenchmarkScanHit32K(b *testing.B) {
	s := benchScanner()
	data := makeHitBody(32 * 1024)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.ScanRequest(data)
	}
}

func BenchmarkScanHTTPFlowReq32K(b *testing.B) {
	s := benchScanner()
	req := makeHitBody(32 * 1024)
	rsp := makeNoisyBody(32 * 1024)
	b.SetBytes(int64(len(req) + len(rsp)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.ScanHTTPFlow("api.example.com", req, rsp)
	}
}

func BenchmarkScanClean1M(b *testing.B) {
	s := benchScanner()
	data := makeNoisyBody(1024 * 1024)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.ScanRequest(data)
	}
}
