package trafficguard

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// simulation_test.go 用本地真实流量历史(~/.yakit-projects)做仿真测试与基准。
// 若本机不存在该数据库, 测试会优雅跳过(不失败), 保证 CI/其他机器可跑。

func defaultHistoryDB() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	candidates := []string{
		filepath.Join(home, "yakit-projects", "default-yakit.db"),
		filepath.Join(home, "yakit-projects", "yakit-default.db"),
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() && st.Size() > 0 {
			return c
		}
	}
	return ""
}

// loadRealFlows 从历史数据库抽取真实 HTTP 流量(请求+响应拼接), 返回样本切片。
// 只取响应长度在合理区间的流量, 避免巨型二进制把测试拖垮。
func loadRealFlows(t testing.TB, limit int) [][]byte {
	t.Helper()
	dbPath := defaultHistoryDB()
	if dbPath == "" {
		t.Skipf("no local yakit history db found, skipping real-corpus simulation")
	}
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Skipf("open sqlite %s failed: %v", dbPath, err)
	}
	defer db.Close()
	rows, err := db.Query(
		`SELECT request || char(10) || char(10) || response FROM http_flows
		 WHERE length(response) BETWEEN 200 AND 200000
		 ORDER BY length(response) DESC LIMIT ?`, limit)
	if err != nil {
		t.Skipf("query http_flows failed (%v); ensure sqlite3 driver available", err)
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var blob string
		if err := rows.Scan(&blob); err == nil && len(blob) > 0 {
			out = append(out, []byte(blob))
		}
	}
	if len(out) == 0 {
		t.Skipf("no usable flows in %s", dbPath)
	}
	return out
}

// TestSimulationRealCorpusNoCrash 用真实历史流量跑扫描, 确保不崩溃、命中合理、零明文落库。
func TestSimulationRealCorpusNoCrash(t *testing.T) {
	s, err := NewScanner()
	if err != nil {
		t.Fatalf("NewScanner failed: %v", err)
	}
	flows := loadRealFlows(t, 60)
	t.Logf("loaded %d real flows from history", len(flows))

	totalHits, scanned := 0, 0
	for _, f := range flows {
		scanned++
		findings := s.ScanRequest(f)
		totalHits += len(findings)
		// 合并 Risk 不应崩溃, 且 details 绝不含命中明文。
		r := BuildMergedRisk(findings, "https://example.test/real", f, nil)
		for _, fd := range findings {
			if r != nil && strings.Contains(r.Details, string(fd.RawValue)) && len(fd.RawValue) > 6 {
				t.Errorf("merged risk details leaked plaintext of a secret")
			}
		}
	}
	t.Logf("simulation: scanned=%d totalHits=%d", scanned, totalHits)
}

// TestSimulationMergedRiskAggregates 真实流量 + 注入命中, 验证一个流量只产出一条合并 Risk。
func TestSimulationMergedRiskAggregates(t *testing.T) {
	s, err := NewScanner()
	if err != nil {
		t.Fatalf("NewScanner failed: %v", err)
	}
	flows := loadRealFlows(t, 1)
	base := flows[0]
	// 在真实流量尾部注入两条不同高危凭证, 模拟同一流量命中多个特征。
	injected := append([]byte(nil), base...)
	injected = append(injected, []byte(" AKIAIOSFODNN7EXAMPLE ghp_"+strings.Repeat("a", 36))...)

	findings := s.ScanRequest(injected)
	r := BuildMergedRisk(findings, "https://api.example.com/v1/login", injected, nil)
	if r == nil {
		t.Fatal("expected merged risk for multi-hit flow")
	}
	// 关键断言: 一个流量只产出一个 Risk 对象。
	if !strings.Contains(r.Title, "敏感凭证泄漏") {
		t.Errorf("title missing keyphrase: %q", r.Title)
	}
	if !strings.Contains(r.Title, "api.example.com") {
		t.Errorf("title missing host: %q", r.Title)
	}
	// 整体等级应取最高(注入了 critical 的 AKIA)。
	if severityForSchema(r.Severity) != "critical" {
		t.Errorf("merged severity should be critical, got %q (raw %q)", severityForSchema(r.Severity), r.Severity)
	}
}

// BenchmarkSimulationRealCorpus 真实流量扫描基准: 证明纯流量下扫描开销极低、不影响实时代理。
func BenchmarkSimulationRealCorpus(b *testing.B) {
	s, err := NewScanner()
	if err != nil {
		b.Fatalf("NewScanner failed: %v", err)
	}
	flows := loadRealFlows(b, 80)
	total := 0
	for _, f := range flows {
		total += len(f)
	}
	b.SetBytes(int64(total) / int64(len(flows)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f := flows[i%len(flows)]
		_ = s.ScanRequest(f)
	}
}
