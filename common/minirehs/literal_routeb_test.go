package minirehs

import (
	"math/rand"
	"regexp/syntax"
	"strings"
	"testing"

	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
)

// TestRe2SupersetRewrite 验证超集改写器对代表性 regexp2 构造的输出可被 RE2 解析, 且对无法
// 安全处理的构造正确 bail.
func TestRe2SupersetRewrite(t *testing.T) {
	cases := []struct {
		in     string
		wantOK bool
	}{
		// 前视/后视断言移除.
		{`foo(?=bar)`, true},
		{`foo(?!bar)x`, true},
		{`(?<=foo)bar`, true},
		{`(?<!foo)bar`, true},
		// \uXXXX -> \x{XXXX}.
		{`[a\u4E00-\u9FFFb]+`, true},
		{`\u0041BC`, true},
		// 原子组 / 命名捕获.
		{`(?>a+)b`, true},
		{`(?<name>abc)d`, true},
		{`(?P<name>abc)d`, true},
		{`(?'name'abc)d`, true},
		// 行内 flag.
		{`(?i)Abc`, true},
		{`(?i:Abc)d`, true},
		// 真实 regexp2-only 规则.
		{`(\b(?![\w]{0,10}?https?://)(([-A-Za-z0-9]{1,20})://[-A-Za-z0-9+&@#/%?=~_|!:,.;]+[-A-Za-z0-9+&@#/%=~_|]))`, true},
		{`(https?://[-A-Za-z0-9+&@#/%?=~_|!:,.;\u4E00-\u9FFF]+[-A-Za-z0-9+&@#/%=~_|])`, true},
		{`(([a-z0-9]+[_|\.])*[a-z0-9]+@([a-z0-9]+[-|_|\.])*[a-z0-9]+\.((?!js|css|jpg|jpeg|png|ico)[a-z]{2,5}))`, true},
		// 反向引用.
		{`(\w+)\1`, true},
		// bail: 悬空反斜杠.
		{`abc\`, false},
		// bail: 未闭合断言.
		{`foo(?=bar`, false},
	}
	for _, c := range cases {
		out, ok := re2Superset(c.in)
		if ok != c.wantOK {
			t.Errorf("re2Superset(%q) ok=%v want=%v (out=%q)", c.in, ok, c.wantOK, out)
			continue
		}
		if ok {
			if _, err := syntax.Parse(out, syntax.Perl); err != nil {
				t.Errorf("re2Superset(%q)=%q not RE2-parsable: %v", c.in, out, err)
			}
		}
	}
}

// TestRouteBExtractKnownRules 断言 route-B 对真实 regexp2-only 规则提取出预期的必需字面量.
func TestRouteBExtractKnownRules(t *testing.T) {
	cases := []struct {
		name string
		expr string
		want []string // 期望提取到的(小写)必需字面量; nil 表示应放弃(always-on)
	}{
		{"参数-URL设计", `(\b(?![\w]{0,10}?https?://)(([-A-Za-z0-9]{1,20})://[-A-Za-z0-9+&@#/%?=~_|!:,.;]+[-A-Za-z0-9+&@#/%=~_|]))`, []string{"://"}},
		{"Url信息", `(https?://[-A-Za-z0-9+&@#/%?=~_|!:,.;\u4E00-\u9FFF]+[-A-Za-z0-9+&@#/%=~_|])`, []string{"http"}},
		{"Email", `(([a-z0-9]+[_|\.])*[a-z0-9]+@([a-z0-9]+[-|_|\.])*[a-z0-9]+\.((?!js|css|jpg|jpeg|png|ico)[a-z]{2,5}))`, nil}, // 仅 "@" 长度 1 < minLen
	}
	for _, c := range cases {
		got := extractRequiredLiteralsApprox(c.expr, 2)
		if !sameStringSet(got, c.want) {
			t.Errorf("%s: got literals=%v want=%v", c.name, got, c.want)
		}
	}
}

// TestRouteBLiteralNecessity 是 route-B 的核心健全性护栏: 对每条 regexp2-only 规则, 若 route-B
// 提取出字面量, 则对大量多样输入断言"regexp2 命中 => 至少一个字面量出现在(小写)输入中"。这是
// "字面量是任意命中的必要条件"的经验验证, 任何非必需字面量都会被它抓到 (从而暴露漏报风险).
func TestRouteBLiteralNecessity(t *testing.T) {
	patterns, names := compilableMITMPatterns(t)
	type rb struct {
		id       PatternID
		expr     string
		literals []string
		v        *regexp2Verifier
	}
	var rbs []rb
	for _, p := range patterns {
		expr := buildExprWithFlags(p)
		// 仅取 regexp2-only (RE2 解析失败) 且 route-B 提到字面量者.
		if _, perr := syntax.Parse(expr, syntax.Perl); perr == nil {
			continue
		}
		lits := extractRequiredLiteralsApprox(expr, 2)
		if len(lits) == 0 {
			continue
		}
		yak := &regexp2Verifier{yak: regexp_utils.NewYakRegexpUtils(expr)}
		rbs = append(rbs, rb{id: p.ID, expr: expr, literals: lits, v: yak})
		t.Logf("route-B gated rule id=%d name=%s literals=%v", p.ID, names[p.ID], lits)
	}
	if len(rbs) == 0 {
		t.Skip("no route-B gated regexp2-only rules")
	}

	check := func(inputs [][]byte) {
		for _, in := range rbs {
			for _, data := range inputs {
				locs := in.v.findAll(data)
				if len(locs) == 0 {
					continue // 不命中, 无需必要性
				}
				low := strings.ToLower(string(data))
				hasLit := false
				for _, l := range in.literals {
					if strings.Contains(low, l) {
						hasLit = true
						break
					}
				}
				if !hasLit {
					t.Fatalf("NECESSITY VIOLATED rule id=%d expr=%q matched %q but no literal %v present (漏报风险)",
						in.id, in.expr, data, in.literals)
				}
			}
		}
	}

	// 1) 真实流量语料.
	records, _ := loadCorpus(t)
	check(records)

	// 2) 随机串 + 结构化 URL/email 片段 (诱发命中, 验证必要性).
	r := rand.New(rand.NewSource(0xB0B))
	tokens := []string{
		"http://a.b/c", "https://x.y/z?q=1", "ftp://no", "://bare", "a://b",
		"user@host.com", "a@b.cn", "name_1@sub.domain.org", "@.", "scheme://h",
		"text http no slash", "HTTPS://UP", "Http://Mixed",
	}
	var randInputs [][]byte
	for i := 0; i < 4000; i++ {
		size := 1 + r.Intn(80)
		var sb strings.Builder
		for sb.Len() < size {
			if r.Intn(4) == 0 {
				sb.WriteString(tokens[r.Intn(len(tokens))])
			} else {
				sb.WriteByte(byte(0x20 + r.Intn(95))) // 可见 ASCII
			}
		}
		randInputs = append(randInputs, []byte(sb.String()))
	}
	check(randInputs)
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ma := map[string]struct{}{}
	for _, x := range a {
		ma[x] = struct{}{}
	}
	for _, x := range b {
		if _, ok := ma[x]; !ok {
			return false
		}
	}
	return true
}
