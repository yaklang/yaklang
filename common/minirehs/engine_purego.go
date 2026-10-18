package minirehs

// engineBackend 是自研多正则引擎 (Tier 2 带 SIMD 预过滤 / Tier 3 标量预过滤).
// 它实现 Hyperscan 式"一次扫描"思想: 对数据先做一次字面量预过滤得到候选 pattern 集,
// 只对候选在数据上做完整正则验证; 无必需字面量的 always-on pattern 每次都需验证.
//
// 关键词: 自研引擎, 字面量预过滤, 候选验证, always-on, 一次扫描
type engineBackend struct{}

func (b *engineBackend) kind() BackendKind { return BackendEngine }

// tier 由构建档位决定: SIMD 预过滤构建为 2, 纯 Go 标量构建为 3.
// 具体取值用 engineTier 常量 (按 build tag 在 prefilter_{cgo,nocgo}.go 中分别定义),
// 避免在共享代码里出现某一构建恒不可达的分支.
func (b *engineBackend) tier() int { return engineTier }

func (b *engineBackend) simd() bool { return simdPrefilterAvailable() }

func (b *engineBackend) compile(patterns []*compiledPattern, cfg *config) (compiledDB, error) {
	db := &engineDB{
		all: patterns,
		n:   len(patterns),
	}

	var withLit []*compiledPattern // 有必要字面量，进候选索引
	for _, cp := range patterns {
		switch {
		case len(cp.literals) > 0:
			withLit = append(withLit, cp)
			db.fullscanLit = append(db.fullscanLit, cp.idx)
		case cp.v != nil && cp.v.exact():
			// 无字面量的 exact (always-on): 恒为候选, 每条记录整段验证.
			db.alwaysOnExact = append(db.alwaysOnExact, cp.idx)
		default:
			// regexp2-only (lookaround/backref): 每条单独验证, 报告存在性命中.
			db.inexact = append(db.inexact, cp)
		}
	}

	if len(withLit) > 0 {
		// withLit 中每条都有必需字面量, 故索引必非空.
		li := buildLiteralIndex(withLit)
		db.pf = newPrefilter(li)
		db.litToPat = li.litToPat
	}
	return db, nil
}

type engineDB struct {
	all      []*compiledPattern // 按 idx 索引
	n        int
	pf       prefilter // 可能为 nil (无任何字面量 pattern)
	litToPat [][]int32 // litID -> pattern idx (与 pf 的字面量索引一致)

	fullscanLit   []int              // 必要字面量触发后，在原完整 subject 上确认一次
	alwaysOnExact []int              // 无字面量的 exact: 每条记录整段验证
	inexact       []*compiledPattern // regexp2-only always-on, 逐条验证
}

// numAlwaysOn 报告"每条记录都要整段参与判定"的 pattern 数 (性能风险提示):
// 无字面量的 exact + regexp2-only.
func (d *engineDB) numAlwaysOn() int { return len(d.alwaysOnExact) + len(d.inexact) }

func (d *engineDB) close() error {
	if r, ok := d.pf.(prefilterReleaser); ok {
		r.release()
	}
	return nil
}

func (d *engineDB) scan(data []byte, sc *scratch, handler MatchHandler) (bool, error) {
	d.resetScratch(sc)
	defer sc.clearVerifierText()

	// 1) 字面量预过滤: 一次扫描得到所有字面量命中 (含位置).
	//    FindAll 的全局非重叠选择不能由独立局部窗口拼接；每候选使用原完整 subject 一次。
	if d.pf != nil {
		hits := d.pf.scanHits(data, sc)
		for _, h := range hits {
			if int(h.litID) >= len(d.litToPat) {
				continue
			}
			for _, pid := range d.litToPat[h.litID] {
				if stop, err := d.verifyFull(data, int(pid), sc, handler); stop || err != nil {
					return stop, err
				}
			}
		}
	}

	// 2) 无字面量的 exact: 每条记录整段验证.
	for _, idx := range d.alwaysOnExact {
		sc.statAlwaysScan++
		cp := d.all[idx]
		for _, loc := range findAllInRecord(cp.v, data, 0, len(data), sc) {
			if !handler(Match{ID: cp.id, From: loc[0], To: loc[1]}) {
				return true, nil
			}
		}
	}

	// 3) regexp2-only always-on: 逐条验证整段数据.
	for _, cp := range d.inexact {
		sc.statAlwaysScan++
		for _, loc := range findAllInRecord(cp.v, data, 0, len(data), sc) {
			if !handler(Match{ID: cp.id, From: loc[0], To: loc[1]}) {
				return true, nil
			}
		}
	}
	return false, nil
}

// verifyFull 对一条候选 pattern 在原完整 subject 上确认，保持全局 FindAll 选择。
// 同一 pattern 在本次扫描内的多个字面量命中只触发一次整段扫描 (fullDone 去重).
func (d *engineDB) verifyFull(data []byte, idx int, sc *scratch, handler MatchHandler) (bool, error) {
	if sc.fullDone[idx] {
		return false, nil
	}
	sc.fullDone[idx] = true
	sc.statFullScan++
	cp := d.all[idx]
	for _, loc := range findAllInRecord(cp.v, data, 0, len(data), sc) {
		if !handler(Match{ID: cp.id, From: loc[0], To: loc[1]}) {
			return true, nil
		}
	}
	return false, nil
}

// resetScratch 复位每次扫描需要清零的复用缓冲 (fullDone / dedup).
func (d *engineDB) resetScratch(sc *scratch) {
	sc.clearVerifierText()
	if cap(sc.fullDone) < d.n {
		sc.fullDone = make([]bool, d.n)
	} else {
		sc.fullDone = sc.fullDone[:d.n]
		for i := range sc.fullDone {
			sc.fullDone[i] = false
		}
	}
	for k := range sc.dedup {
		delete(sc.dedup, k)
	}
}
