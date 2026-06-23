package trafficguard

import (
	"fmt"
	"strings"
	"testing"
)

// Benchmark 证明: 25 条高危超级正则一次编译、一次扫描, 纯净流量与命中流量都极快,
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
		_ = s.ScanHTTPFlow(req, rsp)
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
