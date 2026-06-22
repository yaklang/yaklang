package minirehs

import (
	"testing"
)

// TestMVSConvertibility 诊断 MITM 规则集经 route-B 超集骨架后, 各 pattern 在 MVS 后端的 NFA 归类:
// exact (NFA 权威) / gate (超集门, regexp2 复核) / fallback (仍无 NFA). 用于量化 Phase 1
// "消除 regexp2 税" 的覆盖: 期望 fallback 趋近 0, regexp2 仅在 gate 命中时复核. 仅打印不强断言,
// 真正的正确性护栏是 TestMVSExistenceVsOracleMITM 等差分测试.
func TestMVSConvertibility(t *testing.T) {
	requireDiag(t)
	patterns, names := compilableMITMPatterns(t)

	db, err := Compile(patterns, WithBackend(BackendMVS), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	defer db.Close()
	mdb := digMVSDB2(t, db)

	var exact, gate, assertCnt, fallback int
	for idx, nfa := range mdb.nfas {
		switch {
		case nfa == nil:
			fallback++
			t.Logf("FALLBACK idx=%d name=%s expr=%q", idx, names[mdb.all[idx].id], mdb.all[idx].expr)
		case mdb.gate[idx]:
			gate++
			t.Logf("GATE idx=%d name=%s assert=%v expr=%q", idx, names[mdb.all[idx].id], nfa.hasAssert, mdb.all[idx].expr)
		default:
			exact++
			if nfa.hasAssert {
				assertCnt++
			}
		}
	}
	t.Logf("MVS NFA classification: total=%d exact=%d (assert=%d) gate=%d fallback=%d",
		mdb.n, exact, assertCnt, gate, fallback)

	// 窗口化潜力: withLit (有字面量) 且 windowed (有界宽、无位置锚点) 的 lean NFA 数量.
	// 这些可在字面量命中点邻域窗口内验证, 把 per-trigger existsIn 从 O(record) 降到 O(window).
	var litTotal, litWindowed, litWindowedLean int
	for idx, cp := range mdb.all {
		if len(cp.literals) == 0 {
			continue
		}
		litTotal++
		if cp.windowed {
			litWindowed++
			if nfa := mdb.nfas[idx]; nfa != nil && !nfa.hasAssert && !mdb.gate[idx] {
				litWindowedLean++
			}
		}
	}
	t.Logf("windowing potential: withLit=%d windowed=%d windowed&leanNFA&exact=%d (winW samples below)", litTotal, litWindowed, litWindowedLean)
	shown := 0
	for idx, cp := range mdb.all {
		if len(cp.literals) > 0 && cp.windowed && shown < 8 {
			t.Logf("  windowed idx=%d winW=%d name=%s", idx, cp.winW, names[cp.id])
			shown++
		}
	}
}
