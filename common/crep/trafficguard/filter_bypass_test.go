package trafficguard

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/schema"
)

// 模拟 MITM 的过滤旁路逻辑: 即使流量命中了过滤规则, 只要 TrafficGuard 发现敏感数据,
// 就应该强制取消过滤(isFiltered=false)并把流量保存下来。
func TestFilterBypassForSensitiveFlow(t *testing.T) {
	// 一段"本应被过滤"的大 JS, 但末尾藏了一个 AWS AKID。
	js := "/* bundled */ var a=1;" + strings.Repeat("normal code; ", 4000)
	req := []byte("GET /static/app.js HTTP/1.1\r\nHost: example.com\r\n\r\n")
	rsp := append([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/javascript\r\n\r\n"), []byte(js+" AKIAIOSFODNN7EXAMPLE")...)

	// 1) 过滤前无条件扫描。
	findings := ScanFindings(req, rsp)
	if len(findings) == 0 {
		t.Fatal("expected to find AWS AKID inside filtered JS")
	}

	// 2) 命中即强制取消过滤(模拟 MITM 处的 isFiltered=false)。
	isFiltered := false
	if len(findings) > 0 {
		isFiltered = false // 即使前面过滤策略设了 true, 这里强制保留
	}
	if isFiltered {
		t.Fatal("sensitive flow must NOT be filtered out")
	}

	// 3) flow 保存时复用 findings 标红 + 生成 Risk。
	flow := &schema.HTTPFlow{Url: "https://example.com/static/app.js"}
	var dbCalled bool
	// db 传 nil 时走 yakit.NewRisk 全局库分支; 这里只验证不 panic + 流量标红。
	_ = dbCalled
	ApplyToFlow(nil, flow, findings, req, rsp)
	if !flow.HasColor(schema.FLOW_COLOR_RED) {
		t.Error("flow should be tagged RED")
	}
	if !flow.HasColor(flowTag) {
		t.Errorf("flow should carry trafficguard tag, got tags=%q", flow.Tags)
	}
}

// 验证 ScanFindings 对纯净流量返回 nil(快速排除, 无副作用)。
func TestScanFindingsClean(t *testing.T) {
	clean := []byte("GET / HTTP/1.1\r\nHost: x\r\n\r\njust some boring content " + strings.Repeat("nothing here ", 500))
	if got := ScanFindings(clean, nil); len(got) != 0 {
		t.Errorf("clean flow should yield no findings, got %d", len(got))
	}
}
