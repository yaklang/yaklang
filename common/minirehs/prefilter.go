package minirehs

// literalIndex 是字面量预过滤的与后端无关的编译产物: 去重后的字面量集合, 以及每个
// 字面量到其所属 pattern 下标的映射. NoCGO 与 CGO 两条路径共享它, 只是扫描实现不同.
type literalIndex struct {
	literals []string  // 去重、已 ASCII 小写的字面量
	litToPat [][]int32 // litID -> 该字面量所属的 pattern 下标列表
}

// buildLiteralIndex 从已编译 pattern 集合构建字面量索引. 只纳入"有必需字面量"的 pattern;
// 无字面量的 pattern (always-on) 不进索引, 由引擎单独处理.
func buildLiteralIndex(patterns []*compiledPattern) *literalIndex {
	li := &literalIndex{}
	litID := make(map[string]int32)
	for _, cp := range patterns {
		for _, lit := range cp.literals {
			id, ok := litID[lit]
			if !ok {
				id = int32(len(li.literals))
				litID[lit] = id
				li.literals = append(li.literals, lit)
				li.litToPat = append(li.litToPat, nil)
			}
			li.litToPat[id] = append(li.litToPat[id], int32(cp.idx))
		}
	}
	return li
}

func (li *literalIndex) empty() bool { return len(li.literals) == 0 }

// litHit 是一次字面量预过滤命中: litID 在 data 中结束于 end (exclusive).
type litHit struct {
	litID int32
	end   int32
}

// prefilter 是字面量预过滤策略. scanHits 扫描 data, 返回所有字面量命中 (含位置),
// 供引擎做邻域窗口验证. 实现允许假阳, 绝不假阴, 真伪由后续正则验证判定.
// 返回的切片复用自 sc.hits, 调用方不得跨扫描持有.
type prefilter interface {
	scanHits(data []byte, sc *scratch) []litHit
	simd() bool
}

// prefilterReleaser 由持有本地资源 (如 CGO 内存) 的 prefilter 实现, 供后端在 close 时释放.
type prefilterReleaser interface {
	release()
}

// scalarPrefilter 是纯 Go Aho-Corasick 预过滤 (Tier 3 基线, 全平台可用).
type scalarPrefilter struct {
	ac *ahoCorasick
	li *literalIndex
}

func newScalarPrefilter(li *literalIndex) *scalarPrefilter {
	b := newACBuilder()
	for id, lit := range li.literals {
		b.add(lit, int32(id))
	}
	return &scalarPrefilter{
		ac: b.build(len(li.literals)),
		li: li,
	}
}

func (p *scalarPrefilter) simd() bool { return false }

// release 满足 prefilterReleaser. 纯 Go 标量预过滤无本地资源, 为 no-op;
// 实现它使得 engineDB.close 的释放分支在默认构建下也被一致地走到.
func (p *scalarPrefilter) release() {}

func (p *scalarPrefilter) scanHits(data []byte, sc *scratch) []litHit {
	sc.hits = p.ac.scanHitsFoldASCII(data, sc.hits[:0])
	return sc.hits
}

// asciiLowerInto 把 data 的 ASCII 大写字母转为小写写入复用缓冲 *buf, 返回结果切片.
// 非字母字节与非 ASCII 字节原样保留, 长度不变 (保证偏移一致).
func asciiLowerInto(data []byte, buf *[]byte) []byte {
	if cap(*buf) < len(data) {
		*buf = make([]byte, len(data))
	} else {
		*buf = (*buf)[:len(data)]
	}
	out := *buf
	for i := 0; i < len(data); i++ {
		c := data[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return out
}
