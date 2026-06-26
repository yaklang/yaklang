package minirehs

import (
	"math/rand"
	"regexp/syntax"
	"sort"
	"testing"
)

// buildMergedFor 解析一组 expr, 取其中可编入 NFA 者作为合并成员, 返回合并自动机与成员表.
func buildMergedFor(t *testing.T, exprs []string) (*mvsMergedNFA, []mergeMember) {
	t.Helper()
	var members []mergeMember
	for i, e := range exprs {
		parsed, err := syntax.Parse(e, syntax.Perl)
		if err != nil {
			continue
		}
		nfa, ok := compileMVSNFA(parsed.Simplify())
		if !ok {
			continue
		}
		members = append(members, mergeMember{idx: i, nfa: nfa})
	}
	return buildMergedNFA(members), members
}

// mergedExistSet 跑合并自动机, 返回命中成员 idx 集合.
func mergedExistSet(m *mvsMergedNFA, maxIdx int, data []byte) map[int]struct{} {
	seen := make([]bool, maxIdx+1)
	hits := m.scanExist(data, seen, nil)
	set := make(map[int]struct{}, len(hits))
	for _, idx := range hits {
		set[idx] = struct{}{}
	}
	return set
}

// individualExistSet 各成员单独 existsIn, 返回命中成员 idx 集合 (合并的 oracle).
func individualExistSet(members []mergeMember, data []byte) map[int]struct{} {
	set := make(map[int]struct{})
	for _, mem := range members {
		if mem.nfa.existsIn(data) {
			set[mem.idx] = struct{}{}
		}
	}
	return set
}

func sameIntSet(a, b map[int]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

// TestMVSMergedVsIndividual 验证合并自动机的单趟命中集合, 与各成员单独 existsIn 完全一致.
// 覆盖混合锚定 (^ / $ / 无锚)、单字与多字 (位置数 >64) NFA、ASCII 与多字节.
func TestMVSMergedVsIndividual(t *testing.T) {
	exprs := []string{
		`[0-9]{3,}`,
		`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`,
		`AKIA[0-9A-Z]{16}`,
		`^GET`,
		`END$`,
		`ab+c`,
		`<[^>]+>`,
		`https?://`,
		`[A-Fa-f0-9]{32}`, // 较多位置
		`(foo|bar|baz)`,
		`a[bc]*d`,
		`x{10,40}`, // 展开多位置, 可能跨字
		`你好[0-9]+`, // 多字节
	}
	merged, members := buildMergedFor(t, exprs)
	if merged == nil || len(members) == 0 {
		t.Fatal("no merge members built")
	}
	t.Logf("merged members=%d npos=%d nword=%d nsym=%d", len(members), merged.npos, merged.nword, merged.nsym)

	maxIdx := 0
	for _, mem := range members {
		if mem.idx > maxIdx {
			maxIdx = mem.idx
		}
	}

	r := rand.New(rand.NewSource(0xA11CE))
	tokens := []string{
		"GET /x", "the END", "abc", "abbbbc", "<tag>", "http://a", "https://b",
		"192.168.1.1", "AKIAABCDEFGHIJKLMNOP", "deadBEEF00112233445566778899aabb",
		"12345", "foo", "bar", "baz", "ad", "abcbcd", "你好123", "xxxxxxxxxxxx",
	}
	for i := 0; i < 3000; i++ {
		size := 1 + r.Intn(120)
		data := randomCorpus(r, tokens, size)
		got := mergedExistSet(merged, maxIdx, data)
		ora := individualExistSet(members, data)
		if !sameIntSet(got, ora) {
			t.Fatalf("merged hit set differs on %q: merged=%v individual=%v", data, sortedKeys(got), sortedKeys(ora))
		}
	}
}

// TestMVSMergedMITMRealTraffic 用真实 MITM 规则中"无字面量且可编入 NFA"的那些组成合并自动机,
// 在真实流量每条记录上验证合并命中集合 == 各成员单独 existsIn.
func TestMVSMergedMITMRealTraffic(t *testing.T) {
	patterns, _ := compilableMITMPatterns(t)
	var members []mergeMember
	for _, p := range patterns {
		expr := buildExprWithFlags(p)
		// 仅取无必需字面量 (always-on) 且可编入 NFA 者, 与后端合并集合一致.
		if lits := requiredLiteralsOf(expr); len(lits) > 0 {
			continue
		}
		parsed, err := syntax.Parse(expr, syntax.Perl)
		if err != nil {
			continue
		}
		nfa, ok := compileMVSNFA(parsed.Simplify())
		if !ok {
			continue
		}
		members = append(members, mergeMember{idx: int(p.ID), nfa: nfa})
	}
	merged := buildMergedNFA(members)
	if merged == nil {
		t.Skip("no merge members from MITM rules")
	}
	maxIdx := 0
	for _, mem := range members {
		if mem.idx > maxIdx {
			maxIdx = mem.idx
		}
	}
	t.Logf("merged MITM members=%d npos=%d nword=%d", len(members), merged.npos, merged.nword)

	records, joined := loadCorpus(t)
	if testing.Short() && len(records) > 200 {
		records = records[:200]
	}
	for ri, rec := range records {
		got := mergedExistSet(merged, maxIdx, rec)
		ora := individualExistSet(members, rec)
		if !sameIntSet(got, ora) {
			t.Fatalf("record#%d merged=%v individual=%v", ri, sortedKeys(got), sortedKeys(ora))
		}
	}
	if testing.Short() {
		return
	}
	got := mergedExistSet(merged, maxIdx, joined)
	ora := individualExistSet(members, joined)
	if !sameIntSet(got, ora) {
		t.Fatalf("joined merged=%v individual=%v", sortedKeys(got), sortedKeys(ora))
	}
}

// requiredLiteralsOf 复刻 Compile 的字面量判定 (RE2 优先, 失败走 route-B), 用于在测试里判断
// 某 expr 是否 always-on (无必需字面量).
func requiredLiteralsOf(expr string) []string {
	if parsed, err := syntax.Parse(expr, syntax.Perl); err == nil {
		return extractRequiredLiterals(parsed.Simplify(), 2)
	}
	return extractRequiredLiteralsApprox(expr, 2)
}

func sortedKeys(m map[int]struct{}) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
