package trafficguard

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/schema"
)

// TestRuneSpanMultibyteOffset 复现并锁定"高亮偏移"修复:
//
// 前端高亮(以及 yaklang HookColor)使用 rune(字符)下标, 而 PCRE2 底层接口给出的是 byte 偏移。
// 当命中点之前存在多字节字符(中文注释/字符串等)时, byte 偏移 > rune 下标, 直接落库会让高亮整体右移。
// runeSpan 必须把 byte 偏移换算为 rune 下标, 使前端按 rune 切片时正好覆盖命中明文。
func TestRuneSpanMultibyteOffset(t *testing.T) {
	prefix := "前缀中文注释 token=" // 含多字节字符
	secret := "AKIAIOSFODNN7EXAMPLE"
	buf := []byte(prefix + secret + " 结尾")

	from := strings.Index(string(buf), secret) // byte 偏移
	to := from + len(secret)

	idx, length := runeSpan(buf, from, to)

	wantIdx := utf8.RuneCountInString(prefix)
	if idx != wantIdx {
		t.Errorf("rune index = %d, want %d (= rune count of prefix)", idx, wantIdx)
	}
	// 关键: 含多字节前缀时, rune 下标必须严格小于 byte 偏移, 否则就是旧 bug(右移)。
	if idx >= from {
		t.Errorf("rune index(%d) must be < byte offset(%d) when multibyte chars precede the hit", idx, from)
	}
	if length != utf8.RuneCountInString(secret) {
		t.Errorf("rune length = %d, want %d", length, utf8.RuneCountInString(secret))
	}
	// 模拟前端按 rune 高亮: 用 rune 下标切片必须正好落在命中明文上。
	runes := []rune(string(buf))
	if got := string(runes[idx : idx+length]); got != secret {
		t.Errorf("rune-based highlight slice = %q, want %q (would be misaligned without conversion)", got, secret)
	}
}

// TestRuneSpanAllASCII 验证纯 ASCII 报文下 rune 下标与 byte 偏移一致(不回归常规用例)。
func TestRuneSpanAllASCII(t *testing.T) {
	buf := []byte("HTTP/1.1 200 OK\r\n\r\nakid=AKIAIOSFODNN7EXAMPLE end")
	secret := "AKIAIOSFODNN7EXAMPLE"
	from := strings.Index(string(buf), secret)
	to := from + len(secret)
	idx, length := runeSpan(buf, from, to)
	if idx != from {
		t.Errorf("ascii: rune index(%d) should equal byte offset(%d)", idx, from)
	}
	if length != len(secret) {
		t.Errorf("ascii: rune length(%d) should equal byte length(%d)", length, len(secret))
	}
}

// TestRuneSpanOutOfRange 验证越界/非法偏移做安全退化, 不 panic。
func TestRuneSpanOutOfRange(t *testing.T) {
	buf := []byte("abc")
	if idx, l := runeSpan(buf, -5, 100); idx != 0 || l != 3 {
		t.Errorf("out-of-range should clamp to [0,len): got idx=%d len=%d", idx, l)
	}
	if idx, l := runeSpan(buf, 2, 1); idx != 2 || l != 0 {
		t.Errorf("to<from should yield zero length: got idx=%d len=%d", idx, l)
	}
}

// TestRuleTagsOf 验证命中会生成"具体规则名"TAG(去重、带 TrafficGuard: 前缀, 空名跳过),
// 让用户在 History 能按具体命中的规则筛选, 而非只有笼统的 trafficguard-secret。
func TestRuleTagsOf(t *testing.T) {
	fs := []Finding{
		{RuleID: 2, RuleName: "AWS 访问密钥 ID 泄漏(AKIA/ASIA)"},
		{RuleID: 2, RuleName: "AWS 访问密钥 ID 泄漏(AKIA/ASIA)"}, // 重复, 应去重
		{RuleID: 4, RuleName: "Google API Key 泄漏"},
		{RuleName: "   "}, // 空名应跳过
	}
	tags := ruleTagsOf(fs)
	if len(tags) != 2 {
		t.Fatalf("expected 2 deduped tags, got %d: %v", len(tags), tags)
	}
	want := map[string]bool{
		ruleTagPrefix + "AWS 访问密钥 ID 泄漏(AKIA/ASIA)": true,
		ruleTagPrefix + "Google API Key 泄漏":           true,
	}
	for _, tg := range tags {
		if !want[tg] {
			t.Errorf("unexpected tag %q", tg)
		}
	}
}

// TestFlowRuleTagsApplied 验证(与 ApplyToFlow 同路径)流量会同时打上总括标签与具体规则名标签。
// 这里复用 flow 的 AddTag 系列方法(无 DB 依赖), 锁定 History 可按规则名筛选的行为。
func TestFlowRuleTagsApplied(t *testing.T) {
	fs := []Finding{{RuleID: 4, RuleName: "Google API Key 泄漏"}}
	flow := &schema.HTTPFlow{}
	flow.Red()
	flow.AddTagToFirst(flowTag)
	flow.AddTag(ruleTagsOf(fs)...)

	if !strings.Contains(flow.Tags, flowTag) {
		t.Errorf("flow tags should contain umbrella tag %q, got %q", flowTag, flow.Tags)
	}
	if !strings.Contains(flow.Tags, ruleTagPrefix+"Google API Key 泄漏") {
		t.Errorf("flow tags should contain specific rule-name tag, got %q", flow.Tags)
	}
}
