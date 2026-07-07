package minirehs

import (
	"testing"
)

// BenchmarkMVSBiAnchorAB 精确量化双向锚定 (Rose-lite) 的纯增量: 同一规则+语料, biAnchorEnabled
// ON vs OFF 对照. 全量 (含 regexp2 gate, 被 regexp2 成本稀释) 与 RE2-only (排除 regexp2, 凸显
// 锚定增量) 两档. CGO 构建 C 内核本就快, 增量小; NoCGO 构建无 C 内核, 旧路径纯 Go 整段扫, 增量大.
func BenchmarkMVSBiAnchorAB(b *testing.B) {
	full, _ := compilableMITMPatterns(b)
	re2 := re2OnlyMITMPatterns(b)
	records, _ := loadCorpusB(b)
	run := func(b *testing.B, pats []Pattern, on bool) {
		old := biAnchorEnabled
		biAnchorEnabled = on
		defer func() { biAnchorEnabled = old }()
		benchScanRecordsOpts(b, pats, records, WithBackend(BackendMVS), WithReportLocation(false))
	}
	b.Run("Full/BiAnchorOFF", func(b *testing.B) { run(b, full, false) })
	b.Run("Full/BiAnchorON", func(b *testing.B) { run(b, full, true) })
	b.Run("RE2only/BiAnchorOFF", func(b *testing.B) { run(b, re2, false) })
	b.Run("RE2only/BiAnchorON", func(b *testing.B) { run(b, re2, true) })
}

// TestMVSBiAnchorDispatchDiag 确认双向锚定是否在真实 MITM 三大热点生效, 并量化:
//   - 各 batch-full lean pattern 现归属 (biAnchor / batch / anchor / window)
//   - 仍走 C 内核 batch 的 nfaExists 扫描字节 (cgoNfaExistsBytes), 对照旧整段 winBytes
//
// 仅诊断. 运行: go test -run TestMVSBiAnchorDispatchDiag -v
func TestMVSBiAnchorDispatchDiag(t *testing.T) {
	requireDiag(t)
	patterns, names := compilableMITMPatterns(t)
	records, _ := loadCorpus(t)

	db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	defer db.Close()
	d := digMVSDB(t, db)

	// 1) 资格归属统计.
	var biList, batchList, anchorList, winList []int
	for i := 0; i < d.n; i++ {
		nfa := d.nfas[i]
		if nfa == nil || nfa.hasAssert || d.gate[i] || len(d.all[i].literals) == 0 {
			continue
		}
		switch {
		case d.windowable[i]:
			winList = append(winList, i)
		case d.anchorable[i]:
			anchorList = append(anchorList, i)
		case d.biAnchorable[i]:
			biList = append(biList, i)
		default:
			batchList = append(batchList, i)
		}
	}
	t.Logf("lean pattern 资格: window=%d anchor=%d bi_anchor=%d batch_full=%d",
		len(winList), len(anchorList), len(biList), len(batchList))
	t.Logf("=== biAnchorable patterns ===")
	for _, i := range biList {
		t.Logf("  [bi] %.45s", names[d.all[i].id])
	}
	t.Logf("=== 仍 batch_full (C 整段) patterns ===")
	for _, i := range batchList {
		t.Logf("  [batch] %.45s", names[d.all[i].id])
	}

	// 2) 三大热点定向核验.
	hot := []string{"参数-用户名泄露", "参数-敏感参数(响应)", "参数-密码泄露"}
	for i := 0; i < d.n; i++ {
		nm := names[d.all[i].id]
		for _, h := range hot {
			if nm == h {
				t.Logf("hot %.30s: biAnchorable=%v batchable=%v anchorable=%v revNFA=%v",
					h, d.biAnchorable[i], d.batchable[i], d.anchorable[i], d.revNFAs[i] != nil)
			}
		}
	}

	// 3) 量化 cgo C 内核 batch 扫描字节 (bi-anchoring 应把三大热点移出 C batch -> 字节大降).
	cgoDiagEnabled = true
	cgoNfaExistsBytes, cgoNfaExistsCalls = 0, 0
	cgoMergedBytes, cgoMergedCalls = 0, 0
	defer func() { cgoDiagEnabled = false }()
	scr, _ := db.NewScratch()
	for _, data := range records {
		_ = db.Scan(data, scr, func(m Match) bool { return true })
	}
	t.Logf("cgo nfaExists: calls=%d bytes=%d (对照旧 batch winBytes≈16.35MB)", cgoNfaExistsCalls, cgoNfaExistsBytes)
	t.Logf("cgo merged:    calls=%d bytes=%d", cgoMergedCalls, cgoMergedBytes)
}
