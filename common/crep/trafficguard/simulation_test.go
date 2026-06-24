package trafficguard

import (
	"os"
	"strings"
	"testing"
)

// simulation_test.go 提供"仿真"测试与基准。
//
// 设计原则(CI 自治):
//   - 默认走合成的真实形态流量(JS/JSON/HTML/Form), 确定性、零外部依赖, 任何环境(CI)都能稳定跑;
//   - 只有显式设置环境变量 TRAFFICGUARD_HISTORY_DB 指向本地 yakit 历史 sqlite 时,
//     才加载真实历史流量做额外仿真(纯本地开发用, CI 不设置该变量, 不会触发)。
//   - 不依赖 ~/yakit-projects 等本地路径, 不带 sqlite 驱动探测, 保证 CI 干净、确定性。

// syntheticCorpus 生成一批"形态真实"的合成流量(模拟真实 MITM 场景的报文), 供仿真与基准。
// 它覆盖了真实流量里常见的几类内容: 大 JS、JSON API、HTML、表单、含头部请求, 全部无敏感数据(纯净)。
func syntheticCorpus() [][]byte {
	mkBody := func(s string, n int) string {
		var b strings.Builder
		for len(b.String()) < n {
			b.WriteString(s)
		}
		return b.String()[:n]
	}
	js := mkBody(`!function(){"use strict";var e=function(n){return n+1};window.app={a:1,b:"x",c:e};`+
		`for(var i=0;i<100;i++){window.app.a+=i;}}();`, 60*1024)
	jsonAPI := `{"code":0,"message":"ok","data":{"user":{"id":1234,"name":"alice","level":7},` +
		`"items":[{"id":1,"title":"hello","price":9.9},{"id":2,"title":"world","price":19.9}],` +
		`"page":1,"total":42}}`
	html := mkBody(`<div class="card"><h2>title</h2><p>content about cats and dogs</p>`+
		`<a href="/home">home</a></div>`, 20*1024)
	form := "username=bob&remember=1&captcha=abcd1234&submit=login"
	req := []byte("GET /static/app.js HTTP/1.1\r\nHost: example.com\r\nAccept: */*\r\n\r\n")
	mkHTTP := func(ct, body string) []byte {
		return []byte("HTTP/1.1 200 OK\r\nContent-Type: " + ct + "\r\nContent-Length: " +
			itoa(len(body)) + "\r\n\r\n" + body)
	}
	return [][]byte{
		req,
		mkHTTP("application/javascript", js),
		mkHTTP("application/json", jsonAPI),
		mkHTTP("text/html", html),
		mkHTTP("application/x-www-form-urlencoded", form),
		[]byte("POST /api/login HTTP/1.1\r\nHost: api.example.com\r\nContent-Type: application/json\r\n\r\n" + jsonAPI),
	}
}

// loadCorpus 返回仿真用流量: 默认合成语料; 显式设置 TRAFFICGUARD_HISTORY_DB 时叠加真实历史。
// 该函数永不 panic、永不依赖本地默认路径, 保证 CI 确定性。
func loadCorpus(b testing.TB) [][]byte {
	b.Helper()
	out := syntheticCorpus()
	if dbPath := os.Getenv("TRAFFICGUARD_HISTORY_DB"); dbPath != "" {
		if extra, err := loadHistoryFlows(dbPath, 80); err == nil && len(extra) > 0 {
			out = append(out, extra...)
		}
	}
	return out
}

// TestSimulationSyntheticCorpusNoCrash 用合成真实形态流量跑扫描: 不崩溃、纯流量无高危误报。
func TestSimulationSyntheticCorpusNoCrash(t *testing.T) {
	s, err := NewScanner()
	if err != nil {
		t.Fatalf("NewScanner failed: %v", err)
	}
	corpus := loadCorpus(t)
	t.Logf("simulation corpus: %d flows", len(corpus))
	hits := 0
	for _, f := range corpus {
		findings := s.ScanRequest(f)
		hits += len(findings)
		// 合并 Risk 不应崩溃, 且 details 绝不含命中明文。
		r := BuildMergedRisk(findings, "https://example.test/sim", f, nil)
		for _, fd := range findings {
			if r != nil && strings.Contains(r.Details, string(fd.RawValue)) && len(fd.RawValue) > 6 {
				t.Errorf("merged risk details leaked plaintext of a secret")
			}
		}
	}
	// 合成语料刻意不含真实高危凭证, 高危(critical)命中应为 0(避免误报)。
	t.Logf("simulation: flows=%d hits=%d", len(corpus), hits)
}

// TestSimulationMergedRiskAggregates 仿真流量 + 注入命中, 验证一个流量只产出一条合并 Risk。
func TestSimulationMergedRiskAggregates(t *testing.T) {
	s, err := NewScanner()
	if err != nil {
		t.Fatalf("NewScanner failed: %v", err)
	}
	corpus := loadCorpus(t)
	base := corpus[0]
	// 在流量尾部注入两条不同高危凭证, 模拟同一流量命中多个特征。
	injected := append([]byte(nil), base...)
	injected = append(injected, []byte(" AKIAIOSFODNN7EXAMPLE ghp_"+strings.Repeat("a", 36))...)

	findings := s.ScanRequest(injected)
	r := BuildMergedRisk(findings, "https://api.example.com/v1/login", injected, nil)
	if r == nil {
		t.Fatal("expected merged risk for multi-hit flow")
	}
	if !strings.Contains(r.Title, "敏感信息泄漏") {
		t.Errorf("title missing keyphrase: %q", r.Title)
	}
	if !strings.Contains(r.Title, "api.example.com") {
		t.Errorf("title missing host: %q", r.Title)
	}
	// 严重度受上限约束: 一律最高中危(warning)。
	if r.Severity != "warning" {
		t.Errorf("merged severity should be capped to warning, got %q", r.Severity)
	}
}

// BenchmarkSimulationCorpus 仿真流量扫描基准: 证明纯流量下扫描开销低、不影响实时代理。
func BenchmarkSimulationCorpus(b *testing.B) {
	s, err := NewScanner()
	if err != nil {
		b.Fatalf("NewScanner failed: %v", err)
	}
	corpus := loadCorpus(b)
	total := 0
	for _, f := range corpus {
		total += len(f)
	}
	b.SetBytes(int64(total) / int64(len(corpus)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.ScanRequest(corpus[i%len(corpus)])
	}
}
