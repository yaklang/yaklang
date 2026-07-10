package minirehs

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
)

// TestMVSAnchoredMergedEquivalence 验证 R1-Anchor: scanExistAnchored (merged span-injected
// 单趟扫描) 命中集合 == 各成员单独 existsInAnchored 命中集合. 覆盖随机 pattern + 随机输入 +
// 随机字面量命中 spans (含多字 NFA / 非法 UTF-8 / 空 spans).
func TestMVSAnchoredMergedEquivalence(t *testing.T) {
	// 选一组可编译为 lean NFA 且非 anchoredStart 的 pattern (覆盖 nword==1 和 nword>=2).
	exprs := []string{
		`token=\w+`, `pass[word]?\s*[:=]`, `AKIA[0-9A-Z]{16}`, `a[bc]+d`,
		`https?://[a-z]+`, `[0-9]{4,6}`, `colou?r`, `\d{1,3}\.\d{1,3}`,
		`(GET|POST|PUT)`, `<[^>]+>`, `café`, `[\x{4e00}-\x{9fff}]+`,
		`[a-z]{70}`, `[0-9a-f]{80}`, `(abc|def|ghi){25}`,
	}
	r := rand.New(rand.NewSource(0xA1A1))
	type member struct {
		idx int
		nfa *mvsNFA
	}
	var members []member
	for i, expr := range exprs {
		nfa := buildNFAFor(t, expr)
		if nfa == nil || nfa.anchoredStart || nfa.hasAssert {
			t.Logf("expr=%q skip (nil/anchored/assert)", expr)
			continue
		}
		members = append(members, member{idx: i, nfa: nfa})
	}
	if len(members) < 2 {
		t.Skip("not enough eligible members")
	}

	// 构造 mergeMember 列表 (按 npos 升序, 模拟先导诊断的子集选择).
	mms := make([]mergeMember, len(members))
	for i, m := range members {
		mms[i] = mergeMember{idx: m.idx, nfa: m.nfa}
	}
	sort.Slice(mms, func(i, j int) bool { return mms[i].nfa.npos < mms[j].nfa.npos })

	merged := buildMergedAnchoredNFA(mms)
	if merged == nil {
		t.Fatal("buildMergedAnchoredNFA returned nil")
	}
	t.Logf("anchored merged: members=%d npos=%d nword=%d nsym=%d", len(mms), merged.npos, merged.nword, merged.nsym)

	// 预计算各成员的 prev/cand/active 缓冲 (多字版用).
	prevBufs := make([][]uint64, len(mms))
	candBufs := make([][]uint64, len(mms))
	activeBufs := make([][]uint64, len(mms))
	for i, m := range mms {
		prevBufs[i] = make([]uint64, m.nfa.nword)
		candBufs[i] = make([]uint64, m.nfa.nword)
		activeBufs[i] = make([]uint64, m.nfa.nword)
	}

	checks, mismatches := 0, 0
	for s := 0; s < 500; s++ {
		n := r.Intn(300)
		data := make([]byte, n)
		for i := range data {
			switch r.Intn(3) {
			case 0:
				data[i] = byte("abcdef0123.GETPOST _=?://"[r.Intn(24)])
			case 1:
				data[i] = byte(r.Intn(128))
			default:
				data[i] = byte(r.Intn(256))
			}
		}

		// 为每个成员生成随机 spans (模拟字面量命中).
		spansPerMember := make([][]anchorSpan, len(mms))
		for mi := range mms {
			nspan := r.Intn(4)
			spans := make([]anchorSpan, nspan)
			for j := range spans {
				lo := r.Intn(n + 1)
				hi := lo + r.Intn(n-lo+1)
				spans[j] = anchorSpan{int32(lo), int32(hi)}
			}
			spansPerMember[mi] = mergeAnchorSpans(spans)
		}

		// 各成员单独 existsInAnchored / existsInAnchored1 命中集合.
		indivHits := make(map[int]bool)
		for mi, m := range mms {
			spans := spansPerMember[mi]
			var hit bool
			if m.nfa.single {
				hit = m.nfa.existsInAnchored1(data, spans)
			} else {
				hit = m.nfa.existsInAnchored(data, spans, prevBufs[mi], candBufs[mi], activeBufs[mi])
			}
			if hit {
				indivHits[m.idx] = true
			}
		}

		// merged scanExistAnchored 命中集合.
		seen := make([]bool, len(exprs)+1)
		var out []int
		out = merged.scanExistAnchored(data, spansPerMember, seen, out)
		mergedHits := make(map[int]bool)
		for _, idx := range out {
			mergedHits[idx] = true
		}

		checks++
		if !sameIntSetMap(indivHits, mergedHits) {
			if mismatches < 10 {
				t.Errorf("MISMATCH data=%q\n  indiv=%v\n  merged=%v", string(data), indivHits, mergedHits)
			}
			mismatches++
		}
	}
	if mismatches > 0 {
		t.Fatalf("anchored merged equivalence: %d mismatches in %d checks", mismatches, checks)
	}
	t.Logf("anchored merged equivalence: checks=%d all consistent", checks)
}

// TestMVSAnchoredMergedBackendOracle 把 R1 merged 调度真正接入 backend，在真实规则与
// 真实流量上对照 stdlib。它覆盖 production scan 中 span 收集、成员槽映射、merged hit
// 回报以及未合并的断言成员回退路径；默认开关仍关闭，防止未做性能验收前改变生产选择。
func TestMVSAnchoredMergedBackendOracle(t *testing.T) {
	old := anchorMergedEnabled
	anchorMergedEnabled = true
	defer func() { anchorMergedEnabled = old }()

	patterns, _ := compilableMITMPatterns(t)
	mvs, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false))
	if err != nil {
		t.Fatalf("compile mvs: %v", err)
	}
	defer mvs.Close()
	oracle, err := Compile(patterns, WithBackend(BackendStdlib))
	if err != nil {
		t.Fatalf("compile oracle: %v", err)
	}
	defer oracle.Close()

	records, _ := loadCorpus(t)
	if len(records) > 120 {
		records = records[:120]
	}
	for i, rec := range records {
		got := mvsExistIDs(t, mvs, rec)
		want := mvsExistIDs(t, oracle, rec)
		mvsAssertSameIDSet(t, got, want, fmt.Sprintf("anchored-merged-record#%d", i))
		if t.Failed() {
			t.FailNow()
		}
	}
}

// BenchmarkMVSAnchoredMergedAB 在同一规则集/语料下比较逐条 gap-jump 与 R1 merged
// 调度。单独保留，避免把尚未证明的实现混入主性能数字。
func BenchmarkMVSAnchoredMergedAB(b *testing.B) {
	patterns := re2OnlyMITMPatterns(b)
	records, _ := loadCorpusB(b)
	old := anchorMergedEnabled
	defer func() { anchorMergedEnabled = old }()
	for _, tc := range []struct {
		name    string
		enabled bool
	}{
		{"IndividualGapJump", false},
		{"MergedGapJump", true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			anchorMergedEnabled = tc.enabled
			benchScanRecordsOpts(b, patterns, records, WithBackend(BackendMVS), WithReportLocation(false))
		})
	}
}

func sameIntSetMap(a, b map[int]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}
