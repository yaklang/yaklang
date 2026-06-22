package minirehs

import (
	"testing"
)

// collectLeanMembers 收集一个已编译 mvsDB 里所有 "lean" NFA (非断言) 作为合并成员.
// 用于实验: 把全部可合并 NFA 并成单一自动机, 度量单趟存在性的 npos/nword 与吞吐.
func collectLeanMembers(mdb *mvsDB) []mergeMember {
	var members []mergeMember
	for idx, nfa := range mdb.nfas {
		if nfa != nil && !nfa.hasAssert {
			members = append(members, mergeMember{idx: idx, nfa: nfa})
		}
	}
	return members
}

// BenchmarkMVSAllMergedExist 去风险实验 (Phase 2 主杠杆): 把"全部 lean NFA"并成单一合并自动机,
// 每条报文只过一趟得命中集合 (存在性), 度量吞吐, 并打印 npos/nword 以判断 naive 全并是否会因状态
// 宽度暴涨而变慢. 与 BenchmarkMVSExistence 的"逐模式 existsIn"(2.94 MB/s 天花板, 剔除 regexp2)
// 对照, 验证"单趟全并"能否把吞吐推上一个数量级. 不测 stdlib (太慢, 固定参照 0.17 MB/s).
//
// 关键词: mvscan, all-merged NFA, single-pass, derisk, npos, nword
func BenchmarkMVSAllMergedExist(b *testing.B) {
	patterns, _ := compilableMITMPatterns(b)
	records, _ := loadCorpusB(b)

	db, err := Compile(patterns, WithBackend(BackendMVS), WithLogger(silentLogger{}))
	if err != nil {
		b.Fatalf("compile: %v", err)
	}
	defer db.Close()
	mdb := digMVSDB2(b, db)

	members := collectLeanMembers(mdb)
	merged := buildMergedNFA(members)
	if merged == nil {
		b.Fatal("merged nil")
	}
	b.Logf("all-merged lean members=%d npos=%d nword=%d nsym=%d", len(members), merged.npos, merged.nword, merged.nsym)

	var total int64
	for _, r := range records {
		total += int64(len(r))
	}
	seen := make([]bool, mdb.n)
	out := make([]int, 0, mdb.n)

	b.SetBytes(total)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, rec := range records {
			for j := range seen {
				seen[j] = false
			}
			out = merged.scanExist(rec, seen, out[:0])
		}
	}
	b.StopTimer()
}

// TestMVSLimExVsMerged 差分护栏: LimEx 递推 (mvsLimEx.scanExist) 与朴素合并 (mvsMergedNFA.scanExist)
// 对同一全并成员集, 逐报文 + joined 的命中成员集合必须完全一致 (merged 本身已对照 existsIn/oracle)。
func TestMVSLimExVsMerged(t *testing.T) {
	patterns, _ := compilableMITMPatterns(t)
	db, err := Compile(patterns, WithBackend(BackendMVS), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	defer db.Close()
	mdb := digMVSDB2(t, db)

	merged := buildMergedNFA(collectLeanMembers(mdb))
	if merged == nil {
		t.Fatal("merged nil")
	}
	le := buildLimEx(merged)
	// totalEdges = sum popcount(follow[p]); 最优重排下异常下限 ~= totalEdges - (有后继的位置数)
	// (每个位置至多把一条出边变成 p->p+1 链边)。若该下限 ~= 当前 excCount, 说明重排无收益,
	// 20% 异常是自动机分支结构 (大 alternation / 字符类) 的固有属性。
	totalEdges, posWithSucc := 0, 0
	for p := 0; p < merged.npos; p++ {
		c := 0
		for _, w := range merged.follow[p] {
			c += bitsOnesCount(w)
		}
		totalEdges += c
		if c > 0 {
			posWithSucc++
		}
	}
	excFloor := totalEdges - posWithSucc
	t.Logf("limex: npos=%d nword=%d excCount=%d (%.1f%%) | totalEdges=%d posWithSucc=%d excFloor(optimal reorder)=%d",
		merged.npos, merged.nword, le.excCount, 100*float64(le.excCount)/float64(merged.npos),
		totalEdges, posWithSucc, excFloor)

	records, joined := loadCorpus(t)
	if testing.Short() && len(records) > 300 {
		records = records[:300]
	}
	seenM := make([]bool, mdb.n)
	seenL := make([]bool, mdb.n)
	var outM, outL []int
	cmp := func(data []byte, ctx string) {
		for i := range seenM {
			seenM[i] = false
			seenL[i] = false
		}
		outM = merged.scanExist(data, seenM, outM[:0])
		outL = le.scanExist(data, seenL, outL[:0])
		if !sameIntSliceSet(outM, outL) {
			t.Fatalf("%s: limex hit set != merged (merged=%v limex=%v)", ctx, outM, outL)
		}
	}
	for i, rec := range records {
		cmp(rec, fmtRec(i, len(rec)))
		if t.Failed() {
			t.FailNow()
		}
	}
	cmp(joined, "joined")
}

// BenchmarkMVSLimExAllMergedExist 对照 BenchmarkMVSAllMergedExist: 同一全并成员集, 改用 LimEx 递推,
// 度量"链边左移 + 稀疏异常"相对朴素"逐活跃位置 OR"的吞吐提升 (Phase 3 去风险关键数据)。
func BenchmarkMVSLimExAllMergedExist(b *testing.B) {
	patterns, _ := compilableMITMPatterns(b)
	records, _ := loadCorpusB(b)
	db, err := Compile(patterns, WithBackend(BackendMVS), WithLogger(silentLogger{}))
	if err != nil {
		b.Fatalf("compile: %v", err)
	}
	defer db.Close()
	mdb := digMVSDB2(b, db)
	merged := buildMergedNFA(collectLeanMembers(mdb))
	le := buildLimEx(merged)
	b.Logf("limex all-merged: npos=%d nword=%d excCount=%d", merged.npos, merged.nword, le.excCount)

	var total int64
	for _, r := range records {
		total += int64(len(r))
	}
	seen := make([]bool, mdb.n)
	out := make([]int, 0, mdb.n)
	b.SetBytes(total)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, rec := range records {
			for j := range seen {
				seen[j] = false
			}
			out = le.scanExist(rec, seen, out[:0])
		}
	}
	b.StopTimer()
}

// sameIntSliceSet 比较两个 int 切片作为集合是否相等 (本文件自含, 不依赖 cgo-tagged 测试辅助).
func sameIntSliceSet(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	sa := make(map[int]struct{}, len(a))
	for _, v := range a {
		sa[v] = struct{}{}
	}
	for _, v := range b {
		if _, ok := sa[v]; !ok {
			return false
		}
	}
	return true
}

func bitsOnesCount(w uint64) int {
	c := 0
	for w != 0 {
		w &= w - 1
		c++
	}
	return c
}

func fmtRec(i, n int) string { return "record#" + itoa(i) + "(len=" + itoa(n) + ")" }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	p := len(b)
	for n > 0 {
		p--
		b[p] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		p--
		b[p] = '-'
	}
	return string(b[p:])
}

// digMVSDB2 与 digMVSDB 等价 (从 Database 取内部 *mvsDB), 独立命名避免与其它测试文件重名冲突.
func digMVSDB2(tb testing.TB, db Database) *mvsDB {
	tb.Helper()
	d, ok := db.(*database)
	if !ok {
		tb.Fatalf("db is not *database: %T", db)
	}
	c, ok := d.primary.(*compositeDB)
	if !ok {
		tb.Fatalf("primary is not *compositeDB: %T", d.primary)
	}
	m, ok := c.primary.(*mvsDB)
	if !ok {
		tb.Fatalf("composite.primary is not *mvsDB: %T", c.primary)
	}
	return m
}
