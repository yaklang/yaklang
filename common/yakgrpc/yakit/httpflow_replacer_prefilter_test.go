package yakit

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// 本测试是统一预过滤 Group 的"差分护栏": 核心不变量是——预过滤只会"跳过"规则, 绝不"新增"命中;
// 因此只要保证"任何真正会命中的规则都不会被预过滤跳过"(soundness), 开启预过滤与逐条匹配的最终
// 结果就完全等价。下面在默认规则集 + 刁钻规则 + 含 dechunk 的语料上反复验证该不变量。
// 关键词: MITM replacer prefilter, soundness, 绝不漏报, 差分测试

func loadDefaultMITMRulesForTest(t *testing.T) []*ypb.MITMContentReplacer {
	raw, err := os.ReadFile("../default_mitm_rule")
	if err != nil {
		t.Skipf("cannot read default_mitm_rule fixture: %v", err)
		return nil
	}
	var rules []*ypb.MITMContentReplacer
	if err := json.Unmarshal(raw, &rules); err != nil {
		t.Fatalf("unmarshal default_mitm_rule failed: %v", err)
	}
	if len(rules) == 0 {
		t.Fatalf("default_mitm_rule is empty")
	}
	return rules
}

// trickyRules 覆盖容易踩坑的形态: 行锚 + Multiline、(?i)、ExactMatch、word-boundary、有界字面量门控。
func trickyRules() []*ypb.MITMContentReplacer {
	return []*ypb.MITMContentReplacer{
		{Rule: `(?i)token=\w+`, NoReplace: true, EnableForRequest: true, EnableForResponse: true, EnableForHeader: true, EnableForBody: true, EnableForURI: true, Index: 9001, Color: "yellow"},
		{Rule: `^Authorization:\s*Bearer\s+[A-Za-z0-9._-]+`, NoReplace: true, EnableForRequest: true, EnableForHeader: true, Index: 9002, Color: "red"},
		{Rule: `eyJ[A-Za-z0-9_-]{6,}\.[A-Za-z0-9_-]{6,}\.[A-Za-z0-9_-]{6,}`, NoReplace: true, EnableForRequest: true, EnableForResponse: true, EnableForHeader: true, EnableForBody: true, Index: 9003, Color: "purple"},
		{Rule: `ULTRA_SECRET_LITERAL`, ExactMatch: true, NoReplace: true, EnableForRequest: true, EnableForResponse: true, EnableForHeader: true, EnableForBody: true, Index: 9004, Color: "red"},
		{Rule: `\bsecret\s*=\s*\w+`, NoReplace: true, EnableForRequest: true, EnableForResponse: true, EnableForHeader: true, EnableForBody: true, Index: 9005, Color: "orange"},
		{Rule: `foo\w+bar`, NoReplace: true, EnableForResponse: true, EnableForBody: true, Index: 9006, Color: "green"},
	}
}

type soundCase struct {
	name string
	req  []byte
	rsp  []byte
}

func buildPrefilterCorpus(t *testing.T) []soundCase {
	t.Helper()
	var cases []soundCase

	mkReq := func(name, uri, headers, body string) {
		pkt := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: example.com\r\n%s\r\n%s", uri, headers, body)
		cases = append(cases, soundCase{name: name, req: []byte(pkt)})
	}
	mkRsp := func(name string, body []byte, extraHeader string) {
		head := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n%sContent-Length: %d\r\n\r\n", extraHeader, len(body))
		cases = append(cases, soundCase{name: name, rsp: append([]byte(head), body...)})
	}

	// 命中类: 把默认/刁钻规则会命中的敏感内容放进不同作用域。
	mkReq("uri_token", "/api?token=abc123DEF", "", "")
	mkReq("body_password", "/login", "Content-Type: application/x-www-form-urlencoded\r\nContent-Length: 24\r\n", "username=a&password=secret")
	mkReq("header_bearer", "/x", "Authorization: Bearer eyJabcdef.payload12345.signature9999\r\n", "")
	mkReq("exact_literal", "/x", "X-Note: ULTRA_SECRET_LITERAL here\r\n", "")
	mkReq("secret_eq", "/x", "Content-Length: 18\r\n", "secret=topsecret01")
	mkRsp("rsp_pubkey", []byte("-----BEGIN PUBLIC KEY-----\nMIIBIj...\n-----END PUBLIC KEY-----\n"), "")
	mkRsp("rsp_foo", []byte("noise fooXYZbar noise"), "")
	mkRsp("rsp_jwt", []byte(`{"jwt":"eyJhbGciOi.eyJzdWIiOi.SflKxwRJSM"}`), "")

	// dechunk 路径: chunked 文本响应, 敏感内容跨 chunk 边界, 验证预过滤扫描的是已解码 body。
	chunkedBody := codec.HTTPChunkedEncode([]byte("prefix password=hunter2 suffix"))
	mkRsp("rsp_chunked_pwd", chunkedBody, "Transfer-Encoding: chunked\r\n")

	// gzip 路径: 仅当未被判定为二进制(从而真正进入匹配)时验证; 否则该响应在新旧实现里都不参与匹配。
	if gz, err := utils.GzipCompress([]byte("inner secret=gzipped99 token=gz123")); err == nil {
		mkRsp("rsp_gzip", gz, "Content-Encoding: gzip\r\n")
	}

	// 随机噪声 + 偶发埋点: 制造大量边界, 压测窗口/字面量门控不漏报。
	rng := rand.New(rand.NewSource(0x9e3779b9))
	tokens := []string{"token=ZZ" + randAlnum(rng, 6), "password=" + randAlnum(rng, 8), "secret=" + randAlnum(rng, 5), "ULTRA_SECRET_LITERAL", "foo" + randAlnum(rng, 3) + "bar", ""}
	for i := 0; i < 200; i++ {
		var sb strings.Builder
		sb.WriteString("padding-")
		sb.WriteString(randAlnum(rng, rng.Intn(64)))
		if tk := tokens[rng.Intn(len(tokens))]; tk != "" {
			// 随机插入位置
			s := sb.String()
			pos := rng.Intn(len(s) + 1)
			sb.Reset()
			sb.WriteString(s[:pos])
			sb.WriteString(tk)
			sb.WriteString(s[pos:])
		}
		body := sb.String()
		if i%2 == 0 {
			mkReq(fmt.Sprintf("rand_req_%d", i), "/p?x="+randAlnum(rng, 4), fmt.Sprintf("X-Rand: %s\r\nContent-Length: %d\r\n", randAlnum(rng, 6), len(body)), body)
		} else {
			mkRsp(fmt.Sprintf("rand_rsp_%d", i), []byte(body), "")
		}
	}
	return cases
}

func randAlnum(rng *rand.Rand, n int) string {
	const alpha = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = alpha[rng.Intn(len(alpha))]
	}
	return string(b)
}

// TestMITMReplacerPrefilterSoundness 验证: 任何真正会命中的规则都不会被统一预过滤跳过 (绝不漏报)。
func TestMITMReplacerPrefilterSoundness(t *testing.T) {
	rules := loadDefaultMITMRulesForTest(t)
	rules = append(rules, trickyRules()...)

	prev := MITMReplacerPrefilterEnabled
	MITMReplacerPrefilterEnabled = true
	defer func() { MITMReplacerPrefilterEnabled = prev }()

	replacer := NewMITMReplacer(func() []*ypb.MITMContentReplacer { return rules })
	if replacer.prefilter == nil {
		t.Fatalf("prefilter not built (expected at least some primary rules)")
	}
	t.Logf("prefilter active: size=%d enabledRules=%d", replacer.prefilter.size, len(replacer.rules))

	corpus := buildPrefilterCorpus(t)

	var (
		totalChecks  int
		totalSkipped int // 被预过滤跳过的(规则,报文)对; 用于确认预过滤确实在干活
		totalMatched int
	)
	for _, c := range corpus {
		skipResp := shouldSkipResponseRuleMatch(c.rsp)
		reqInfo, rspInfo, mask := replacer.prepareColorMatch(c.req, c.rsp, skipResp)
		if mask == nil {
			t.Fatalf("case %s: candidate mask is nil while prefilter active", c.name)
		}
		for _, rule := range replacer.rules {
			if rule == nil || (!rule.EnableForRequest && !rule.EnableForResponse) {
				continue
			}
			totalChecks++

			skipped := rule.prefilterID >= 0 && !mask[rule.prefilterID]

			// 基准真值: 在共享的、已切分的 info 上直接匹配 (绕过预过滤)。
			matched := false
			if rule.EnableForRequest && reqInfo != nil {
				if res, err := rule.MatchByPacketInfo(reqInfo); err == nil && len(res) > 0 {
					matched = true
				}
			}
			if !matched && rule.EnableForResponse && rspInfo != nil {
				if res, err := rule.MatchByPacketInfo(rspInfo); err == nil && len(res) > 0 {
					matched = true
				}
			}
			if matched {
				totalMatched++
			}
			if skipped {
				totalSkipped++
			}

			if matched && skipped {
				t.Fatalf("UNSOUND: case %q rule[%d] %q matched by regexp2 but was filtered out by prefilter (prefilterID=%d)",
					c.name, rule.Index, truncRuleExpr(rule.Rule), rule.prefilterID)
			}
		}
	}

	t.Logf("checks=%d matched=%d prefiltered_skips=%d (skip ratio=%.1f%%)",
		totalChecks, totalMatched, totalSkipped, float64(totalSkipped)/float64(totalChecks)*100)
	if totalSkipped == 0 {
		t.Fatalf("prefilter skipped nothing across the corpus; it is not doing any useful work")
	}
}

// TestMITMReplacerPrefilterEquivalence 端到端等价: 对同一批报文, 开启/关闭预过滤时 appendHookColorExtractions
// 产出的提取数据应完全一致 (条数 + 每条 Data/Index/Length/IsMatchRequest 一致)。
func TestMITMReplacerPrefilterEquivalence(t *testing.T) {
	rules := loadDefaultMITMRulesForTest(t)
	rules = append(rules, trickyRules()...)
	corpus := buildPrefilterCorpus(t)

	collect := func(enabled bool) map[string][]string {
		prev := MITMReplacerPrefilterEnabled
		MITMReplacerPrefilterEnabled = enabled
		defer func() { MITMReplacerPrefilterEnabled = prev }()

		replacer := NewMITMReplacer(func() []*ypb.MITMContentReplacer { return rules })
		out := make(map[string][]string)
		for _, c := range corpus {
			skipResp := shouldSkipResponseRuleMatch(c.rsp)
			reqInfo, rspInfo, mask := replacer.prepareColorMatch(c.req, c.rsp, skipResp)
			var sigs []string
			for _, rule := range replacer.rules {
				if rule == nil || (!rule.EnableForRequest && !rule.EnableForResponse) {
					continue
				}
				if mask != nil && rule.prefilterID >= 0 && !mask[rule.prefilterID] {
					continue
				}
				if rule.EnableForRequest && reqInfo != nil {
					if res, err := rule.MatchByPacketInfo(reqInfo); err == nil {
						for _, r := range res {
							sigs = append(sigs, fmt.Sprintf("REQ|%d|%s", rule.Index, r.MatchResult))
						}
					}
				}
				if rule.EnableForResponse && rspInfo != nil {
					if res, err := rule.MatchByPacketInfo(rspInfo); err == nil {
						for _, r := range res {
							sigs = append(sigs, fmt.Sprintf("RSP|%d|%s", rule.Index, r.MatchResult))
						}
					}
				}
			}
			out[c.name] = sigs
		}
		return out
	}

	on := collect(true)
	off := collect(false)
	if len(on) != len(off) {
		t.Fatalf("case count mismatch on=%d off=%d", len(on), len(off))
	}
	for name, offSigs := range off {
		onSigs := on[name]
		if !sameStringMultiset(onSigs, offSigs) {
			t.Fatalf("case %q: prefilter ON/OFF results diverge\n on=%v\noff=%v", name, onSigs, offSigs)
		}
	}
}

// BenchmarkMITMReplacerColorPath 对比"开启/关闭统一预过滤"时染色/提取路径每报文的匹配开销。
// 运行: go test ./common/yakgrpc/yakit/ -run x -bench MITMReplacerColorPath -benchmem
func BenchmarkMITMReplacerColorPath(b *testing.B) {
	raw, err := os.ReadFile("../default_mitm_rule")
	if err != nil {
		b.Skipf("cannot read default_mitm_rule: %v", err)
	}
	var rules []*ypb.MITMContentReplacer
	if err := json.Unmarshal(raw, &rules); err != nil {
		b.Fatalf("unmarshal: %v", err)
	}

	// 贴近真实流量: 多数报文不含敏感词 (预过滤收益最大), 少量含敏感词。
	benign := []byte("GET /static/app.js?v=20260101 HTTP/1.1\r\nHost: cdn.example.com\r\nUser-Agent: Mozilla/5.0\r\nAccept: */*\r\nReferer: https://example.com/home\r\n\r\n")
	sensitive := []byte("POST /api/login HTTP/1.1\r\nHost: example.com\r\nAuthorization: Bearer eyJabc.def123.sig456\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: 30\r\n\r\nuser=admin&password=hunter2xx!")
	corpus := [][]byte{benign, benign, benign, benign, sensitive}

	run := func(b *testing.B, enabled bool) {
		prev := MITMReplacerPrefilterEnabled
		MITMReplacerPrefilterEnabled = enabled
		defer func() { MITMReplacerPrefilterEnabled = prev }()
		replacer := NewMITMReplacer(func() []*ypb.MITMContentReplacer { return rules })
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			pkt := corpus[i%len(corpus)]
			reqInfo, _, mask := replacer.prepareColorMatch(pkt, nil, true)
			for _, rule := range replacer.rules {
				if rule == nil || !rule.EnableForRequest {
					continue
				}
				if mask != nil && rule.prefilterID >= 0 && !mask[rule.prefilterID] {
					continue
				}
				_, _ = rule.MatchByPacketInfo(reqInfo)
			}
		}
	}

	b.Run("prefilter_off", func(b *testing.B) { run(b, false) })
	b.Run("prefilter_on", func(b *testing.B) { run(b, true) })
}

func truncRuleExpr(s string) string {
	if len(s) <= 80 {
		return s
	}
	return s[:80] + "..."
}

func sameStringMultiset(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]int, len(a))
	for _, s := range a {
		m[s]++
	}
	for _, s := range b {
		m[s]--
		if m[s] < 0 {
			return false
		}
	}
	return true
}
