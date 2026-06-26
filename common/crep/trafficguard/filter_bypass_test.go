package trafficguard

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/schema"
)

// 模拟 MITM 的过滤旁路逻辑: 即使流量命中了过滤规则, 只要 TrafficGuard 发现敏感数据,
// 该流量也不应被直接丢弃, 而是以"插件流量"(source_type=scan + FromPlugin)形式保存(不进 MITM TAB)。
func TestFilterBypassForSensitiveFlow(t *testing.T) {
	// 一段"本应被过滤"的大 JS, 但末尾藏了一个 AWS AKID。
	js := "/* bundled */ var a=1;" + strings.Repeat("normal code; ", 4000)
	req := []byte("GET /static/app.js HTTP/1.1\r\nHost: example.com\r\n\r\n")
	rsp := append([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/javascript\r\n\r\n"), []byte(js+" AKIAIOSFODNN7EXAMPLE")...)

	// 1) 过滤前无条件扫描(带 target 上下文用于第三阶段校验)。
	findings := ScanFindings("https://example.com/static/app.js", req, rsp)
	if len(findings) == 0 {
		t.Fatal("expected to find AWS AKID inside filtered JS")
	}

	// 2) 模拟 MITM 决策: 本应被过滤(isFiltered=true) + 命中 -> 以插件流量形式保存。
	isFiltered := true
	tgSaveAsPlugin := isFiltered && len(findings) > 0
	if !tgSaveAsPlugin {
		t.Fatal("filtered sensitive flow must be kept as plugin-typed flow")
	}

	// 3) flow 保存: 设插件流量标记后, 复用 findings 标红 + 标注 + 生成 Risk。
	flow := &schema.HTTPFlow{Url: "https://example.com/static/app.js"}
	if tgSaveAsPlugin {
		flow.SourceType = schema.HTTPFlow_SourceType_SCAN
		flow.FromPlugin = PluginName
	}
	// db 传 nil 时走 yakit.NewRisk 全局库分支; 这里只验证不 panic + 流量标红/标注。
	ApplyToFlow(nil, flow, findings, req, rsp)
	if !flow.HasColor(schema.FLOW_COLOR_RED) {
		t.Error("flow should be tagged RED")
	}
	if !flow.HasColor(flowTag) {
		t.Errorf("flow should carry trafficguard tag, got tags=%q", flow.Tags)
	}
	// 插件流量: 不进 MITM History(source_type=mitm) TAB。
	if flow.SourceType != schema.HTTPFlow_SourceType_SCAN || flow.FromPlugin == "" {
		t.Errorf("filtered+hit flow should be saved as plugin-typed flow, got source=%q fromPlugin=%q", flow.SourceType, flow.FromPlugin)
	}
	// Payload 应写入命中内容。
	if flow.Payload == "" {
		t.Error("flow.Payload should be populated with hit content")
	}
}

// 验证 ScanFindings 对纯净流量返回 nil(快速排除, 无副作用)。
func TestScanFindingsClean(t *testing.T) {
	clean := []byte("GET / HTTP/1.1\r\nHost: x\r\n\r\njust some boring content " + strings.Repeat("nothing here ", 500))
	if got := ScanFindings("http://x/", clean, nil); len(got) != 0 {
		t.Errorf("clean flow should yield no findings, got %d", len(got))
	}
}
