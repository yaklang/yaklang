package minirehs

import (
	"regexp"
	"sort"
	"testing"
	"time"
)

// TestDiagEngineHotspot 统计引擎在真实语料上的热点: 窗口验证次数 / 触发的整段验证次数 /
// always-on 整段扫描次数, 以及 always-on 单独耗时, 用于定位真正瓶颈.
func TestDiagEngineHotspot(t *testing.T) {
	if testing.Short() {
		t.Skip("diagnostic test, skipped in -short")
	}
	patterns := re2OnlyMITMPatternsT(t)
	records, _ := loadCorpus(t)

	db, err := Compile(patterns, WithBackend(BackendEngine), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	scI, _ := db.NewScratch()
	sc := scI.(*scratch)

	start := time.Now()
	for _, rec := range records {
		_ = db.Scan(rec, sc, func(Match) bool { return true })
	}
	total := time.Since(start)
	t.Logf("engine total over corpus: %v", total)
	t.Logf("windowVerify=%d fullScan(triggered)=%d alwaysScan=%d",
		sc.statWindowVerify, sc.statFullScan, sc.statAlwaysScan)

	eng := db.(*database).primary.(*compositeDB).primary.(*engineDB)
	t.Logf("fullscanLit=%d alwaysOnExact=%d inexact=%d",
		len(eng.fullscanLit), len(eng.alwaysOnExact), len(eng.inexact))

	// 单独测 always-on (无字面量 exact + regexp2-only) 子集的耗时.
	aoIDs := make(map[PatternID]struct{})
	for _, idx := range eng.alwaysOnExact {
		aoIDs[eng.all[idx].id] = struct{}{}
	}
	for _, cp := range eng.inexact {
		aoIDs[cp.id] = struct{}{}
	}
	var aoPat []Pattern
	for _, p := range patterns {
		if _, ok := aoIDs[p.ID]; ok {
			aoPat = append(aoPat, p)
		}
	}
	if len(aoPat) > 0 {
		aoDB, _ := Compile(aoPat, WithBackend(BackendStdlib), WithLogger(silentLogger{}))
		defer aoDB.Close()
		aoSc, _ := aoDB.NewScratch()
		start = time.Now()
		for _, rec := range records {
			_ = aoDB.Scan(rec, aoSc, func(Match) bool { return true })
		}
		t.Logf("always-on subset (%d patterns) alone: %v", len(aoPat), time.Since(start))
	}

	// 列出"有字面量但非窗口" (整段验证) 的 pattern 及原因.
	t.Logf("=== non-windowed-with-literal patterns (cause full scans) ===")
	for _, cp := range eng.all {
		if len(cp.literals) > 0 && !cp.windowed {
			var expr string
			for _, p := range patterns {
				if p.ID == cp.id {
					expr = p.Expr
				}
			}
			_, parsed, _ := compileAndParse(buildExprWithFlags(Pattern{ID: cp.id, Expr: expr}))
			w, bounded := maxByteWidth(parsed)
			anchor := hasPositionAnchor(parsed)
			t.Logf("id=%d bounded=%v width=%d anchor=%v lits=%v expr=%q",
				cp.id, bounded, w, anchor, cp.literals, shorten(expr))
		}
	}
}

// TestDiagBottleneck 诊断真实语料上的瓶颈: always-on 数量、平均候选数、最慢的若干 pattern.
func TestDiagBottleneck(t *testing.T) {
	if testing.Short() {
		t.Skip("diagnostic test, skipped in -short")
	}
	patterns := re2OnlyMITMPatternsT(t)
	records, _ := loadCorpus(t)

	// 统计 always-on 与字面量分布.
	cfg := newDefaultConfig()
	var alwaysOn, withLit int
	type litInfo struct {
		id   PatternID
		lits []string
	}
	var infos []litInfo
	for _, p := range patterns {
		expr := buildExprWithFlags(p)
		_, parsed, err := compileAndParse(expr)
		if err != nil {
			continue
		}
		lits := extractRequiredLiterals(parsed, cfg.minLiteralLen)
		if len(lits) == 0 {
			alwaysOn++
		} else {
			withLit++
		}
		infos = append(infos, litInfo{p.ID, lits})
	}
	t.Logf("patterns=%d withLiteral=%d alwaysOn=%d", len(patterns), withLit, alwaysOn)

	// 列出 always-on 的 pattern (这些每条记录都要全量跑).
	for _, in := range infos {
		if len(in.lits) == 0 {
			for _, p := range patterns {
				if p.ID == in.id {
					t.Logf("ALWAYS-ON id=%d expr=%q", p.ID, shorten(p.Expr))
				}
			}
		}
	}

	// 测每条 pattern 单独跑完整语料的耗时, 找最慢的.
	type tt struct {
		id PatternID
		d  time.Duration
	}
	var times []tt
	for _, p := range patterns {
		re, err := regexp.Compile(buildExprWithFlags(p))
		if err != nil {
			continue
		}
		start := time.Now()
		for _, rec := range records {
			_ = re.FindAllIndex(rec, -1)
		}
		times = append(times, tt{p.ID, time.Since(start)})
	}
	sort.Slice(times, func(i, j int) bool { return times[i].d > times[j].d })
	t.Logf("=== slowest 12 patterns over full corpus (single-pattern) ===")
	for i := 0; i < len(times) && i < 12; i++ {
		var expr string
		for _, p := range patterns {
			if p.ID == times[i].id {
				expr = p.Expr
			}
		}
		t.Logf("#%d id=%d %v expr=%q", i+1, times[i].id, times[i].d, shorten(expr))
	}
}

func shorten(s string) string {
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}

func re2OnlyMITMPatternsT(t *testing.T) []Pattern {
	all, _ := mitmPatterns(t)
	var out []Pattern
	for _, p := range all {
		if _, _, err := compileAndParse(buildExprWithFlags(p)); err == nil {
			out = append(out, p)
		}
	}
	return out
}
