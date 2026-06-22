package minirehs

import (
	"regexp/syntax"
)

// mvsBackend 是自托管 mvscan 引擎的纯 Go 参考实现 (M1): 把每条 RE2 pattern 编译为字节级
// Glushkov 位并行 NFA, 扫描时做存在性判定 (命中 From/To=-1). 无法编入 NFA 的 pattern
// (中缀锚点/词边界/regexp2-only 等) 退回 verifier 兜底, 保证不丢规则、与 oracle 一致.
//
// 它既是 mvscan C 后端的可执行规格 (差分参照), 也用于量化"纯算法 (无 SIMD/无 cgo)"的加速比.
// C 加速档 (SSE/AVX2/NEON) 在后续里程碑以 build tag 接入, 并以本实现做逐位对照.
//
// 关键词: mvscan, BackendMVS, Glushkov, bit-parallel, existence, 纯 Go 参考实现
type mvsBackend struct{}

func (b *mvsBackend) kind() BackendKind { return BackendMVS }
func (b *mvsBackend) tier() int         { return 1 }
func (b *mvsBackend) simd() bool        { return false }

func (b *mvsBackend) compile(patterns []*compiledPattern, cfg *config) (compiledDB, error) {
	db := &mvsDB{
		all:  patterns,
		n:    len(patterns),
		nfas: make([]*mvsNFA, len(patterns)),
	}

	var withLit []*compiledPattern // 有必需字面量 (nfa 或兜底): 进字面量索引, 命中才验证
	nfaCount := 0
	for _, cp := range patterns {
		if nfa := tryCompileNFA(cp); nfa != nil {
			db.nfas[cp.idx] = nfa
			nfaCount++
		}
		if len(cp.literals) > 0 {
			withLit = append(withLit, cp)
		} else {
			db.alwaysOn = append(db.alwaysOn, cp.idx)
		}
	}

	if len(withLit) > 0 {
		li := buildLiteralIndex(withLit)
		db.pf = newPrefilter(li)
		db.litToPat = li.litToPat
	}
	db.nfaCount = nfaCount
	cfg.logger.Infof("minirehs/mvs compiled %d pattern(s): nfa=%d fallback=%d always_on=%d",
		db.n, nfaCount, db.n-nfaCount, len(db.alwaysOn))
	return db, nil
}

// tryCompileNFA 尝试把一条 pattern 编入字节级 NFA; 返回 nil 表示应走 verifier 兜底.
func tryCompileNFA(cp *compiledPattern) *mvsNFA {
	if cp.v == nil || !cp.v.exact() {
		return nil // regexp2-only (lookaround/backref): 自动机不可表达
	}
	parsed, err := syntax.Parse(cp.expr, syntax.Perl)
	if err != nil {
		return nil
	}
	nfa, ok := compileMVSNFA(parsed.Simplify())
	if !ok {
		return nil
	}
	return nfa
}

type mvsDB struct {
	all      []*compiledPattern // 按 idx 索引
	n        int
	nfas     []*mvsNFA // 按 idx; nil 表示该 pattern 走 verifier 兜底
	nfaCount int

	pf       prefilter // 可能为 nil (无任何字面量 pattern)
	litToPat [][]int32 // litID -> pattern idx
	alwaysOn []int     // 无字面量 pattern (nfa 或兜底): 每条记录都参与判定
}

func (d *mvsDB) numAlwaysOn() int { return len(d.alwaysOn) }

func (d *mvsDB) close() error {
	if r, ok := d.pf.(prefilterReleaser); ok {
		r.release()
	}
	return nil
}

func (d *mvsDB) scan(data []byte, sc *scratch, handler MatchHandler) (bool, error) {
	d.resetScratch(sc)

	// 1) 字面量预过滤: 命中某 pattern 的必需字面量, 才对其做一次存在性验证.
	if d.pf != nil {
		hits := d.pf.scanHits(data, sc)
		for _, h := range hits {
			if int(h.litID) >= len(d.litToPat) {
				continue
			}
			for _, idx := range d.litToPat[h.litID] {
				if sc.fullDone[idx] {
					continue
				}
				sc.fullDone[idx] = true
				if stop := d.verifyOne(int(idx), data, handler); stop {
					return true, nil
				}
			}
		}
	}

	// 2) 无字面量 pattern: 每条记录都验证.
	for _, idx := range d.alwaysOn {
		if sc.fullDone[idx] {
			continue
		}
		sc.fullDone[idx] = true
		if stop := d.verifyOne(idx, data, handler); stop {
			return true, nil
		}
	}
	return false, nil
}

// verifyOne 对一条 pattern 做存在性判定: 有 NFA 走位并行扫描, 否则走 verifier 兜底.
// 命中以 From/To=-1 上报 (存在性语义). 返回 true 表示 handler 要求停止.
func (d *mvsDB) verifyOne(idx int, data []byte, handler MatchHandler) bool {
	cp := d.all[idx]
	if nfa := d.nfas[idx]; nfa != nil {
		if nfa.existsIn(data) {
			if !handler(Match{ID: cp.id, From: -1, To: -1}) {
				return true
			}
		}
		return false
	}
	// 兜底: verifier 命中即存在.
	if len(cp.v.findAll(data)) > 0 {
		if !handler(Match{ID: cp.id, From: -1, To: -1}) {
			return true
		}
	}
	return false
}

func (d *mvsDB) resetScratch(sc *scratch) {
	if cap(sc.fullDone) < d.n {
		sc.fullDone = make([]bool, d.n)
	} else {
		sc.fullDone = sc.fullDone[:d.n]
		for i := range sc.fullDone {
			sc.fullDone[i] = false
		}
	}
}
