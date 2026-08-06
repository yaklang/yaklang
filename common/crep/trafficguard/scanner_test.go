package trafficguard

import (
	"math/rand"
	"strings"
	"testing"
)

func TestNewScannerCompilesAllRules(t *testing.T) {
	s, err := NewScanner()
	if err != nil {
		t.Fatalf("NewScanner failed: %v", err)
	}
	if s.Len() != len(builtinRules) {
		t.Fatalf("rule count mismatch: got %d want %d", s.Len(), len(builtinRules))
	}
	if !s.Ready() {
		t.Fatal("scanner not ready")
	}
	t.Logf("rules=%d alwaysOn=%d (existence phase), extract phase=pcre2", s.Len(), s.NumAlwaysOn())
	// All MVS candidates are mandatory literals. Rule 18 has a dedicated fixed
	// shape gate, so no full-record always-on NFA should remain.
	if s.NumAlwaysOn() != 0 {
		t.Fatalf("alwaysOn=%d should be zero", s.NumAlwaysOn())
	}
}

func TestDiscordCandidateGate(t *testing.T) {
	valid := []byte("M" + strings.Repeat("a", 23) + "." + strings.Repeat("b", 6) + "." + strings.Repeat("c", 27))
	if !hasDiscordTokenCandidate(valid) {
		t.Fatal("valid Discord token shape was rejected by candidate gate")
	}
	invalid := append([]byte(nil), valid...)
	invalid[24] = '-'
	if hasDiscordTokenCandidate(invalid) {
		t.Fatal("invalid Discord token shape passed candidate gate")
	}
}

func TestDiscordCandidateGateMatchesLegacyOracle(t *testing.T) {
	random := rand.New(rand.NewSource(0x59414b))
	alphabet := []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-.!@#$%^&*()[]{}\x00\xff")
	valid := []byte("M" + strings.Repeat("a", 23) + "." + strings.Repeat("b", 6) + "." + strings.Repeat("c", 27))
	for iteration := 0; iteration < 10000; iteration++ {
		data := make([]byte, random.Intn(1024))
		for index := range data {
			data[index] = alphabet[random.Intn(len(alphabet))]
		}
		if len(data) >= len(valid) && iteration%4 == 0 {
			start := random.Intn(len(data) - len(valid) + 1)
			copy(data[start:], valid)
			if iteration%8 == 0 {
				data[start] = 'N'
			}
		}
		want := legacyDiscordTokenCandidateForBenchmark(data)
		if got := hasDiscordTokenCandidate(data); got != want {
			t.Fatalf("candidate mismatch at iteration %d: got %v want %v data=%q", iteration, got, want, data)
		}
	}
}

func TestCandidateGateCoversEveryExactRule(t *testing.T) {
	discord := "M" + strings.Repeat("a", 23) + "." + strings.Repeat("b", 6) + "." + strings.Repeat("c", 27)
	fixtures := map[int]string{
		1:  "-----BEGIN RSA PRIVATE KEY-----",
		2:  "AKIAIOSFODNN7EXAMPLE",
		3:  "aws_secret_access_key=" + strings.Repeat("a", 40),
		4:  "AIza" + strings.Repeat("a", 35),
		5:  "ya29." + strings.Repeat("a", 20),
		6:  "AccountKey=" + strings.Repeat("a", 50),
		7:  "ghp_" + strings.Repeat("a", 36),
		8:  "glpat-" + strings.Repeat("a", 20),
		9:  "xoxb-" + strings.Repeat("a", 20),
		10: "https://hooks.slack.com/services/TAAAAAA/BBBBBBB/" + strings.Repeat("a", 16),
		11: "sk_live_" + strings.Repeat("a", 24),
		12: "sk-proj-" + strings.Repeat("a", 40),
		13: "SG." + strings.Repeat("a", 22) + "." + strings.Repeat("b", 43),
		14: "SK" + strings.Repeat("a", 32),
		15: "key-" + strings.Repeat("a", 32),
		16: "sq0atp-" + strings.Repeat("a", 22),
		17: "amzn.mws.12345678-1234-1234-1234-123456789abc",
		18: discord,
		19: "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0." + strings.Repeat("a", 16),
		22: "mysql://root:password@127.0.0.1/database",
		23: `"password":"example-secret"`,
		24: "api_key=" + strings.Repeat("a", 20),
		25: "X-API-Key: " + strings.Repeat("a", 20),
	}

	scanner, err := NewScanner()
	if err != nil {
		t.Fatal(err)
	}
	for index, rule := range scanner.rules {
		fixture, ok := fixtures[rule.ID]
		if !ok {
			t.Fatalf("missing candidate fixture for rule %d", rule.ID)
		}
		matches, matchErr := scanner.extractors[index].FindAll([]byte(fixture), -1)
		if matchErr != nil || len(matches) == 0 {
			t.Fatalf("fixture does not satisfy exact rule %d: matches=%d err=%v", rule.ID, len(matches), matchErr)
		}
		candidateIndexes := scanner.candidateRules([]byte(fixture))
		found := false
		for _, candidateIndex := range candidateIndexes {
			if candidateIndex == index {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("candidate gate missed exact rule %d", rule.ID)
		}
	}
}

func TestScanHighRiskSecrets(t *testing.T) {
	s, err := NewScanner()
	if err != nil {
		t.Fatalf("NewScanner failed: %v", err)
	}
	cases := []struct {
		name   string
		data   string
		ruleID int
		// asResponse: 在响应方向扫描(JWT 仅响应方向保留, 见 validators.go validateJWT)。
		asResponse bool
	}{
		{name: "aws-akia", data: "config aws key AKIAIOSFODNN7EXAMPLE and more", ruleID: 2},
		{name: "google-api", data: "key=AIzaSyDQ5Z4oX9pV2mN7bR3tK6cY1fH8eJ4gU5wX", ruleID: 4},
		{name: "github", data: "token ghp_abcdefghijklmnopqrstuvwxyz0123456789AB ok", ruleID: 7},
		{name: "gitlab", data: "glpat-" + strings.Repeat("x", 20), ruleID: 8},
		// 注: 用拼接构造测试 token, 避免源码中出现形似真实密钥的字面量(触发 push protection)。
		// "s"+"k"+"_live_"+zeros 仍满足 (?:sk|rk)_live_[0-9a-zA-Z]{24,}
		{name: "stripe", data: "secret " + "s" + "k" + "_live_" + strings.Repeat("0", 24), ruleID: 11},
		// openai 的 "s"+"k-" 前缀同样拼接构造, 避免触发 push protection。
		{name: "openai", data: "s" + "k-proj-" + strings.Repeat("a", 60), ruleID: 12},
		{name: "openai-legacy", data: "s" + "k-" + strings.Repeat("a", 48), ruleID: 12},
		{name: "sendgrid", data: "SG." + strings.Repeat("a", 22) + "." + strings.Repeat("b", 43), ruleID: 13},
		{name: "twilio", data: "SK" + strings.Repeat("a", 32), ruleID: 14},
		{name: "mailgun", data: "key-" + strings.Repeat("a", 32), ruleID: 15},
		{name: "square", data: "sq0atp-" + strings.Repeat("a", 22), ruleID: 16},
		// JWT 仅在响应方向 + 首段含 alg 的真实 JWT header 时保留(请求方向视为第一方会话凭证抑制)。
		{name: "jwt", data: "token=eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c", ruleID: 19, asResponse: true},
		{name: "db-conn", data: "mysql://root:pass1234@10.0.0.1:3306/db", ruleID: 22},
		{name: "password-field", data: `{"password":"SuperSecret123"}`, ruleID: 23},
		{name: "api-key-param", data: "https://api.example.com/data?api_key=ak_live_1234567890abcdef", ruleID: 24},
		{name: "x-api-key", data: "X-API-Key: " + strings.Repeat("a1B2", 8), ruleID: 25},
		{name: "pem-key", data: "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\n-----END RSA PRIVATE KEY-----", ruleID: 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var fs []Finding
			if c.asResponse {
				fs = s.ScanResponse([]byte(c.data))
			} else {
				fs = s.ScanRequest([]byte(c.data))
			}
			hit := false
			for _, f := range fs {
				if f.RuleID == c.ruleID {
					hit = true
					if f.MaskedValue == "" {
						t.Errorf("rule %d masked value empty", c.ruleID)
					}
					if f.Fingerprint == "" {
						t.Errorf("rule %d fingerprint empty", c.ruleID)
					}
					// 脱敏后不应包含完整明文(私钥/连接串全脱敏; 这里只做弱校验)。
					break
				}
			}
			if !hit {
				t.Errorf("expected rule %d to match %q; got findings=%v", c.ruleID, c.data, summarize(fs))
			}
		})
	}
}

func TestScanNoFalsePositiveOnNoise(t *testing.T) {
	s, err := NewScanner()
	if err != nil {
		t.Fatalf("NewScanner failed: %v", err)
	}
	// 纯噪声文本不应命中高危规则。
	noise := []string{
		"hello world, this is a normal page about cats and dogs",
		"the quick brown fox jumps over the lazy dog 1234567890",
		"Lorem ipsum dolor sit amet, consectetur adipiscing elit.",
		strings.Repeat("normal text without secrets ", 200),
	}
	for _, n := range noise {
		if fs := s.ScanRequest([]byte(n)); len(fs) > 0 {
			t.Errorf("noise produced unexpected findings: %v", summarize(fs))
		}
	}
}

func TestDedup(t *testing.T) {
	in := []Finding{
		{RuleID: 7, Direction: "request", Fingerprint: "a"},
		{RuleID: 7, Direction: "request", Fingerprint: "a"}, // dup
		{RuleID: 7, Direction: "response", Fingerprint: "a"},
		{RuleID: 2, Direction: "request", Fingerprint: "a"},
	}
	out := Dedup(in)
	if len(out) != 3 {
		t.Fatalf("dedup count: got %d want 3 (%v)", len(out), out)
	}
}

func TestRedact(t *testing.T) {
	if got := redact("AKIAIOSFODNN7EXAMPLE", 4, 2); !strings.HasPrefix(got, "AKIA") {
		t.Errorf("expected head preserved, got %q", got)
	}
	// 私钥全脱敏。
	if got := redact("-----BEGIN PRIVATE KEY-----", 0, 0); strings.Contains(got, "PRIVATE") {
		t.Errorf("private key should be fully redacted, got %q", got)
	}
	// 过短不暴露全部。
	if got := redact("abc", 2, 2); strings.Contains(got, "abc") {
		t.Errorf("short secret should be fully redacted, got %q", got)
	}
}

func summarize(fs []Finding) string {
	var b strings.Builder
	for _, f := range fs {
		b.WriteString(itoa(f.RuleID))
		b.WriteString(":")
		b.WriteString(f.RuleName)
		b.WriteString(" ")
	}
	return b.String()
}

func TestBuildMergedRiskOneFlowOneRisk(t *testing.T) {
	s, err := NewScanner()
	if err != nil {
		t.Fatalf("NewScanner failed: %v", err)
	}
	// 同一流量命中 AKIA(critical) + GitHub(critical) + password(warning) 多条规则。
	data := []byte("akid=AKIAIOSFODNN7EXAMPLE token=ghp_" + strings.Repeat("a", 36) + ` {"password":"P@ss1234"}`)
	findings := s.ScanRequest(data)
	if len(findings) < 2 {
		t.Fatalf("expected multi-hit, got %d", len(findings))
	}
	r := BuildMergedRisk(findings, "https://api.example.com/login", data, nil)
	if r == nil {
		t.Fatal("expected merged risk")
	}
	// 一个流量只产出一个 Risk 对象。
	if !strings.Contains(r.Title, "敏感信息泄漏") {
		t.Errorf("title: %q", r.Title)
	}
	if !strings.Contains(r.Title, "api.example.com") {
		t.Errorf("title missing host: %q", r.Title)
	}
	// 严重度受上限约束: trafficguard Risk 一律最高中危(warning), 即便命中 critical 级特征。
	if r.Severity != "warning" {
		t.Errorf("severity want warning (capped) got %q", r.Severity)
	}
	// details(机器可读)仍只含脱敏值, 不含完整明文。
	if strings.Contains(r.Details, "AKIAIOSFODNN7EXAMPLE") {
		t.Error("details leaked AKIA plaintext")
	}
	// description(markdown)应给出命中值与上下文, 便于人工判真假。
	if !strings.Contains(r.Description, "命中值") {
		t.Errorf("description should contain hit value section: %q", r.Description)
	}
}
