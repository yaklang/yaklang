package bruteutils

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// 真实互联网目标爆破鲁棒性测试（YAK_BRUTE_SHODAN=1 + SHODAN_API_KEY 启用）。
//
// 目的：在真实互联网服务上验证爆破探针的错误分类质量——绝大多数尝试
// 都应得到明确的「认证失败」，而非协议错误、超时或 panic；同时积累
// 各协议在真实服务端版本/配置分布下的错误反馈。
//
// 规模与限速（对未授权目标保持最小影响）：
//   - 每协议最多 YAK_BRUTE_SHODAN_TARGETS（默认 6）个目标；
//   - 每目标仅 2 次明显无效的凭证尝试（不存在的用户名）；
//   - 全局串行 + 每次尝试间隔 ≥1s。
//
//	 SHODAN_API_KEY=... YAK_BRUTE_SHODAN=1 go test -run TestShodanRealWorld -v -timeout 2h
//

const shodanAttemptInterval = time.Second

type shodanMatch struct {
	IPStr     string `json:"ip_str"`
	Port      int    `json:"port"`
	Transport string `json:"transport"`
}

func shodanSearch(t *testing.T, key, query string, limit int) []string {
	t.Helper()
	u := fmt.Sprintf("https://api.shodan.io/shodan/host/search?key=%s&query=%s&minify=false", key, urlQueryEscape(query))
	client := &http.Client{Timeout: 30 * time.Second}
	if p := os.Getenv("YAK_BRUTE_SHODAN_PROXY"); p != "" {
		if pu, err := url.Parse(p); err == nil {
			client.Transport = &http.Transport{Proxy: http.ProxyURL(pu)}
		}
	}
	req, _ := http.NewRequest("GET", u, nil)
	// 瞬时网络抖动重试（本地 fake-ip 代理偶发 reset）
	var resp *http.Response
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		resp, err = client.Do(req)
		if err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		t.Skipf("shodan search %q unavailable: %v", query, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("shodan search %q: HTTP %d: %s", query, resp.StatusCode, truncate(string(body), 200))
	}
	var parsed struct {
		Matches []shodanMatch `json:"matches"`
		Total   int           `json:"total"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("shodan decode: %v", err)
	}
	var out []string
	for _, m := range parsed.Matches {
		if m.Transport != "tcp" || m.IPStr == "" || m.Port <= 0 {
			continue
		}
		out = append(out, fmt.Sprintf("%s:%d", m.IPStr, m.Port))
		if len(out) >= limit {
			break
		}
	}
	t.Logf("shodan %q: total=%d, sampled=%d", query, parsed.Total, len(out))
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func urlQueryEscape(s string) string {
	return url.QueryEscape(s)
}

// classifyNetError 把网络/协议错误归入稳定类别，用于统计分布。
func classifyNetError(err error) string {
	if err == nil {
		return "nil"
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "connection refused"):
		return "conn-refused"
	case strings.Contains(s, "i/o timeout"), strings.Contains(s, "timeout"):
		return "timeout"
	case strings.Contains(s, "connection reset"), strings.Contains(s, "broken pipe"):
		return "conn-reset"
	case strings.Contains(s, "eof"):
		return "eof"
	case strings.Contains(s, "no route"), strings.Contains(s, "network unreachable"), strings.Contains(s, "unreachable"):
		return "no-route"
	case strings.Contains(s, "tls"), strings.Contains(s, "handshake"):
		return "tls"
	case strings.Contains(s, "protocol"):
		return "protocol-mismatch"
	case strings.Contains(s, "auth"), strings.Contains(s, "access denied"), strings.Contains(s, "authentication"):
		return "auth-rejected"
	case strings.Contains(s, "context canceled"):
		return "cancelled"
	default:
		return "other"
	}
}

// probeLegacyHandler 通过旧 handler 执行一次探测，返回 (ok, finished, errclass, elapsed)。
// rdp 额外从底层调用拿详细错误用于分类（handler 结果不带错误串）；
// 其余协议以 handler 的 ok/finished 二元结果分类，不重复探测。
func shodanProbeLegacy(t *testing.T, proto, target string) (ok, finished bool, class string, elapsed time.Duration) {
	t.Helper()
	handler := authFunc[proto]
	if handler == nil {
		t.Fatalf("no handler for %s", proto)
	}
	item := &BruteItem{
		Type:     proto,
		Target:   target,
		Username: "yak-nosuchuser-x1",
		Password: "yak-wrongpass-x1",
		Context:  context.Background(),
	}
	start := time.Now()
	res := handler.BrutePass(item)
	elapsed = time.Since(start)

	if proto == "rdp" {
		if host, portStr, ok2 := splitHostPort(target); ok2 {
			_, err := rdpLoginContext(item.Context, host, host, item.Username, item.Password, atoiDefault(portStr, 3389))
			if err != nil {
				class = classifyNetError(err)
				return res.Ok, res.Finished, class, elapsed
			}
		}
		class = "nil"
		return res.Ok, res.Finished, class, elapsed
	}
	if res.Ok {
		class = "auth-ok"
	} else if res.Finished {
		class = "target-unreachable-or-protocol"
	} else {
		class = "auth-rejected"
	}
	return res.Ok, res.Finished, class, elapsed
}

func splitHostPort(target string) (host, port string, ok bool) {
	idx := strings.LastIndex(target, ":")
	if idx <= 0 || idx == len(target)-1 {
		return "", "", false
	}
	return target[:idx], target[idx+1:], true
}
func atoiDefault(s string, def int) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	if n == 0 {
		return def
	}
	return n
}

func TestShodanRealWorldBrute(t *testing.T) {
	if os.Getenv("YAK_BRUTE_SHODAN") != "1" {
		t.Skip("set YAK_BRUTE_SHODAN=1 + SHODAN_API_KEY to run against internet targets")
	}
	key := os.Getenv("SHODAN_API_KEY")
	if key == "" {
		t.Fatal("SHODAN_API_KEY is empty")
	}
	limit := 6
	if v := os.Getenv("YAK_BRUTE_SHODAN_TARGETS"); v != "" {
		limit = atoiDefault(v, 6)
	}

	protocols := []struct {
		name  string
		query string
	}{
		{name: "rdp", query: "port:3389"},
		{name: "mysql", query: "port:3306"},
		{name: "postgres", query: "port:5432"},
		{name: "mssql", query: "port:1433"},
		{name: "mongodb", query: "port:27017"},
		{name: "ssh", query: "port:22"},
		{name: "ftp", query: "port:21"},
		{name: "redis", query: "port:6379"},
		{name: "telnet", query: "port:23"},
		{name: "memcached", query: "port:11211"},
		{name: "vnc", query: "port:5900"},
		{name: "smb", query: "port:445"},
	}

	var mu sync.Mutex
	histogram := map[string]map[string]int{} // proto → class → count
	var totalAttempts, totalOK int

	for _, proto := range protocols {
		t.Run(proto.name, func(t *testing.T) {
			targets := shodanSearch(t, key, proto.query, limit)
			if len(targets) == 0 {
				t.Skip("no tcp targets")
			}
			mu.Lock()
			histogram[proto.name] = map[string]int{}
			mu.Unlock()

			for _, tgt := range targets {
				ok, finished, class, elapsed := shodanProbeLegacy(t, proto.name, tgt)
				mu.Lock()
				histogram[proto.name][fmt.Sprintf("ok=%v,finished=%v,err=%s", ok, finished, class)]++
				totalAttempts++
				if ok {
					totalOK++
				}
				mu.Unlock()
				t.Logf("%-10s %-22s ok=%-5v finished=%-5v errclass=%-16s %v",
					proto.name, tgt, ok, finished, class, elapsed.Round(100*time.Millisecond))
				time.Sleep(shodanAttemptInterval)
			}
		})
	}

	t.Run("report", func(t *testing.T) {
		t.Log("==== 真实目标错误分类直方图 ====")
		names := make([]string, 0, len(histogram))
		for k := range histogram {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, proto := range names {
			classes := make([]string, 0)
			for c := range histogram[proto] {
				classes = append(classes, c)
			}
			sort.Strings(classes)
			var parts []string
			n := 0
			for _, c := range classes {
				parts = append(parts, fmt.Sprintf("%s×%d", c, histogram[proto][c]))
				n += histogram[proto][c]
			}
			t.Logf("%-10s (%d attempts): %s", proto, n, strings.Join(parts, ", "))
		}
		t.Logf("total attempts=%d, unexpected ok=%d", totalAttempts, totalOK)
	})
}
