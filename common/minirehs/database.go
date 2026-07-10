package minirehs

import (
	"regexp"
	"regexp/syntax"

	"github.com/yaklang/yaklang/common/utils"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
)

// Scratch 是每次扫描需要的可复用工作区, 用于逼近热路径低分配.
// 每个 goroutine 应独占一份 Scratch (非并发安全).
type Scratch interface {
	Close() error
}

// scratch 是 Scratch 的内部实现, 持有可复用缓冲区.
type scratch struct {
	lower      []byte                // ASCII 小写化数据缓冲 (供大小写无关的字面量预过滤复用)
	hits       []litHit              // 字面量预过滤命中缓冲 (含位置)
	cpairs     []int32               // CGO 预过滤输出的 (end,litID) 对缓冲 (NoCGO 不使用)
	dedup      map[matchKey]struct{} // 邻域窗口验证的去重集合 (跨多次命中)
	fullDone   []bool                // 非窗口候选 pattern 是否已做过整段验证 (按 idx)
	mergedHits []int                 // mvs 合并 always-on 自动机单趟命中的成员 idx 缓冲 (复用)

	// mvs 合并 always-on 单趟扫描的"成员级去重"缓冲 (按成员 idx 去重, 不触碰 fullDone;
	// 跨步去重由调用方用 fullDone 完成). mergedSeen 供纯 Go 路径, cseen/cmerged 供 C 内核路径.
	mergedSeen []bool  // 纯 Go scanExist 的成员去重位图 (长度 npat)
	cseen      []byte  // C 合并 scan 的去重位图 (uint8, 长度 npat)
	cmerged    []int32 // C 返回命中成员 idx 的 int32 缓冲

	// mvs 存在性快路径"按报文批处理 cgo"缓冲 (Phase 2): batchIdx 收集本报文触发的、可走 C 内核
	// per-pattern 存在性的 pattern idx (去重), 一次 cgo 调用 nfaExistsMany 后, batchOut[i] 回写
	// 各 idx 命中 (1/0). 把每报文 cgo 次数从 O(触发数) 降到 O(1), 摊薄跨界开销.
	batchIdx []int32
	batchOut []byte

	// mvs 存在性本地化 (Rose-lite) 的每报文窗口累积缓冲 (按 idx): batchSeen 标记该 idx 是否已入批,
	// winLo/winHi 为其本报文全部字面量命中点窗口的 union (覆盖任一匹配, 见 mvs_window.go). 入批后
	// 用收窄子切片 data[winLo:winHi] 做一次 C 存在性门控, 把整段 O(record) 降到 O(window).
	batchSeen []bool
	winLo     []int32
	winHi     []int32

	// 断言 NFA 共享边界缓冲: 一个报文内多条零宽断言 NFA (\b \B / 行锚 等) 复用同一份"逐字节
	// 边界条件" (computeBoundaries 产物), 把 boundaryConds / isWordRune / DecodeRune 的逐 pattern
	// 重复计算降为每报文一次. assertBoundReady 标记本报文是否已算 (惰性, 无断言触发则不算).
	assertBound      []uint8
	assertBoundReady bool

	// gateBound 是"超集门局部化复核"的零宽断言边界缓冲: gate 命中字面量后在 data[winLo:] 子切片上
	// 跑断言超集预检需逐切片重算边界 (winLo 随报文不同), 故不与 assertBound 共享; 复用底层数组减分配。
	gateBound []uint8

	// 锚定式单趟 (anchored single-pass) 每报文缓冲: anchorSeen 标记某 idx 是否已入锚定批,
	// anchorRanges[idx] 累积其全部命中点的注入区间 (批后 mergeAnchorSpans 排序合并),
	// anchorBatch 收集本报文触发的锚定式 pattern idx. anchorPrev/Cand/Active 为锚定执行器复用的
	// 位并行状态缓冲 (长度 = 全部锚定 pattern 的 max nword), 避免热路径分配.
	anchorSeen   []bool
	anchorRanges [][]anchorSpan
	anchorBatch  []int32
	anchorPrev   []uint64
	anchorCand   []uint64
	anchorActive []uint64

	// C 锚定批处理的平铺视图。每条 lean pattern 的已合并 spans 连续写入
	// anchorSpansLo/Hi，anchorSpanOff 划分各 pattern 的子区间；避免在热路径为
	// []anchorSpan -> C 平行数组做逐条分配/跨界。
	anchorCIdx     []int32
	anchorSpanOff  []int32
	anchorSpansLo  []int32
	anchorSpansHi  []int32
	anchorBatchOut []byte

	// R1 anchored merged scan 的每报文成员 span 视图。元素只借用 anchorRanges 中
	// 已合并的切片，不复制 span；扫描结束后下次 reset 时覆盖。
	anchorMergedSpans [][]anchorSpan

	// 双向锚定 (Rose-lite 完全体) 每报文缓冲: biSeen 标记某 idx 是否已入双向锚定批; biFwdRanges[idx]
	// 累积前向注入区间 [h.end-headF, h.end] (头有界字面量), biRevRanges[idx] 累积反向注入区间
	// [h.end, h.end+tailR] (尾有界字面量); biBatch 收集本报文触发的双向锚定 pattern idx. 位并行状态
	// 复用 anchorPrev/Cand/Active (前向锚定与反向锚定顺序执行, 可共用缓冲).
	biSeen      []bool
	biFwdRanges [][]anchorSpan
	biRevRanges [][]anchorSpan
	biBatch     []int32

	// 定位 (findLocFrom / findAllLoc) 的位并行状态缓冲: locPrev/locCand 长度 = NFA nword,
	// locCandStart/locPrevStart 长度 = NFA npos (每活跃 position 的起点字节偏移). 定位被
	// finalizeHit 每命中调用, 旧版每调用 make 四个切片 (位置模式 alloc 大头); 改为复用本缓冲,
	// 按需增长. 缓冲内每元素均"写后读" (见 findLocFrom 注释), 无需逐次清零, 故零初始化开销.
	locPrev      []uint64
	locCand      []uint64
	locCandStart []int
	locPrevStart []int

	// 诊断计数 (仅供测试观察热点, 每个 scratch 独占, 无并发竞争).
	statWindowVerify int64 // 邻域窗口验证次数
	statFullScan     int64 // 非窗口 exact (有字面量) 命中字面量后触发的整段验证次数
	statAlwaysScan   int64 // 无字面量 exact + regexp2-only 的逐条整段扫描次数
}

func (s *scratch) Close() error { return nil }

// Database 是编译产物, 不可变、并发安全 (只读), 可被多 goroutine 共享.
type Database interface {
	// NewScratch 分配一份与该 db 绑定的可复用扫描临时区.
	NewScratch() (Scratch, error)
	// Scan 对完整 data 做 block 扫描, 每命中一次调用 handler; handler 返回 false 提前终止.
	Scan(data []byte, s Scratch, handler MatchHandler) error
	// Info 返回该 db 的元信息.
	Info() DatabaseInfo
	// Close 释放后端持有的本地资源 (纯 Go 后端为 no-op).
	Close() error
}

// compiledPattern 是一条经过特性 gate、字面量提取后的内部表示.
type compiledPattern struct {
	id       PatternID
	idx      int      // 在 supported 集合内的下标
	expr     string   // 已带 flag 前缀的最终表达式 (供组合 gate 复用, 保证语义一致)
	v        verifier // 命中判定与取偏移 (引擎与 oracle 共享, 保证一致)
	literals []string // 必需字面量 OR 集 (已 ASCII 小写); 为空表示 always-on
	windowed bool     // 是否可在字面量命中点的邻域窗口内验证 (有界宽度且无位置锚点)
	winW     int      // 邻域验证窗口的半宽 (字节), 即正则最大可能宽度
}

// backendImpl 是后端的内部契约 (不对外暴露).
type backendImpl interface {
	kind() BackendKind
	tier() int
	simd() bool
	compile(patterns []*compiledPattern, cfg *config) (compiledDB, error)
}

// compiledDB 是某后端编译出的可扫描实例.
type compiledDB interface {
	// scan 返回 stopped=true 表示 handler 主动要求停止.
	scan(data []byte, sc *scratch, handler MatchHandler) (stopped bool, err error)
	numAlwaysOn() int
	close() error
}

// Compile 把一组 patterns 编译成不可变 Database. 默认 opts 走 Auto 后端探测.
// 编译昂贵但一次性; 返回的 Database 可被多 goroutine 并发只读使用.
//
// 关键词: minirehs.Compile, 多正则统一编译, compile then scan
func Compile(patterns []Pattern, opts ...Option) (Database, error) {
	cfg := newDefaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	if len(patterns) == 0 {
		return nil, utils.Error("minirehs: no patterns to compile")
	}

	var (
		supported []*compiledPattern
		reports   = make([]PatternReport, 0, len(patterns))
	)

	for _, p := range patterns {
		if p.Expr == "" {
			return nil, utils.Errorf("minirehs: pattern id=%d has empty expr", p.ID)
		}

		expr := buildExprWithFlags(p)

		// 用 yaklang regexp-utils 做特性 gate: 优先标准库 RE2, 失败时回退 regexp2
		// (支持 lookaround/backref). 二者都无法编译才视为不支持.
		yak := regexp_utils.NewYakRegexpUtils(expr)
		if !yak.CanUse() {
			policy := effectivePolicy(p.OnUnsupported, cfg.defaultPolicy)
			reason := "expression not compilable by RE2 nor regexp2"
			if policy == Fallback {
				// stdlib/regexp2 都不能编译, Fallback 无从降级.
				cfg.logger.Warnf("minirehs: pattern id=%d unsupported and rejected: %s", p.ID, reason)
			}
			reports = append(reports, PatternReport{ID: p.ID, Disposition: "rejected", Reason: reason})
			return nil, utils.Errorf("minirehs: pattern id=%d rejected: %s", p.ID, reason)
		}

		cp := &compiledPattern{id: p.ID, expr: expr}

		// 尝试用标准库 RE2 编译并做字面量分析; 成功则走精确验证 + 预过滤路径.
		if re, parsed, err := compileAndParse(expr); err == nil {
			cp.v = &re2Verifier{re: re}
			cp.literals = extractRequiredLiterals(parsed, cfg.minLiteralLen)
			if len(cp.literals) > 0 {
				if w, ok := windowVerifiable(parsed); ok {
					cp.windowed = true
					cp.winW = w
				}
			}
		} else {
			// RE2 不可表达 (lookaround/backref 等), 用 regexp2 验证 (后端已全局切 go-pcre2-lite).
			cp.v = &regexp2Verifier{yak: yak}
			// route-B: 在不触碰 regexp2 AST 的前提下, 用"语言超集改写 + RE2 字面量提取"取必需字面量,
			// 命中才验证, 避免每条记录都跑昂贵的 regexp2. 提不出则保持 always-on. (健全性见 literal_routeb.go)
			cp.literals = extractRequiredLiteralsApprox(expr, cfg.minLiteralLen)
		}

		supported = append(supported, cp)
		reports = append(reports, PatternReport{
			ID:          p.ID,
			Disposition: dispositionOf(cp),
			HasLiteral:  len(cp.literals) > 0,
		})
	}

	for i, cp := range supported {
		cp.idx = i
	}

	backend, err := selectBackend(cfg)
	if err != nil {
		return nil, err
	}
	primary, err := backend.compile(supported, cfg)
	if err != nil {
		return nil, err
	}

	info := DatabaseInfo{
		Backend:     backend.kind(),
		Tier:        backend.tier(),
		SIMD:        backend.simd(),
		Composite:   false,
		NumPatterns: len(patterns),
		NumAlwaysOn: primary.numAlwaysOn(),
		Reports:     reports,
	}

	cfg.logger.Infof("minirehs compiled %d pattern(s): backend=%s tier=%d simd=%v always_on=%d",
		info.NumPatterns, info.Backend, info.Tier, info.SIMD, info.NumAlwaysOn)

	return &database{primary: newCompositeDB(primary, nil), info: info}, nil
}

// database 是对外的 Database 实现, 包裹具体后端的 compiledDB.
type database struct {
	primary compiledDB
	info    DatabaseInfo
}

func (d *database) NewScratch() (Scratch, error) {
	return &scratch{
		lower:    make([]byte, 0, 4096),
		hits:     make([]litHit, 0, 256),
		dedup:    make(map[matchKey]struct{}, 64),
		fullDone: make([]bool, d.info.NumPatterns),
	}, nil
}

func (d *database) Scan(data []byte, s Scratch, handler MatchHandler) error {
	sc, ok := s.(*scratch)
	if !ok || sc == nil {
		ns, err := d.NewScratch()
		if err != nil {
			return err
		}
		sc = ns.(*scratch)
	}
	if handler == nil {
		handler = func(Match) bool { return true }
	}
	_, err := d.primary.scan(data, sc, handler)
	return err
}

func (d *database) Info() DatabaseInfo { return d.info }

func (d *database) Close() error { return d.primary.close() }

// buildExprWithFlags 把 Pattern.Flags 映射为 RE2 行内标志前缀.
func buildExprWithFlags(p Pattern) string {
	var fl []byte
	if p.Flags&FlagCaseless != 0 {
		fl = append(fl, 'i')
	}
	if p.Flags&FlagDotAll != 0 {
		fl = append(fl, 's')
	}
	if p.Flags&FlagMultiline != 0 {
		fl = append(fl, 'm')
	}
	if len(fl) == 0 {
		return p.Expr
	}
	return "(?" + string(fl) + ")" + p.Expr
}

// compileAndParse 同时返回可执行的 *regexp.Regexp 与用于字面量分析的语法树.
// 二者都基于标准库 (RE2/syntax.Perl), 保证验证语义与 stdlib oracle 完全一致.
func compileAndParse(expr string) (*regexp.Regexp, *syntax.Regexp, error) {
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, nil, err
	}
	parsed, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		return nil, nil, err
	}
	return re, parsed.Simplify(), nil
}

func effectivePolicy(p, def UnsupportedPolicy) UnsupportedPolicy {
	if p == DefaultPolicy {
		if def == DefaultPolicy {
			return Reject
		}
		return def
	}
	return p
}

func dispositionOf(cp *compiledPattern) string {
	if cp.v != nil && !cp.v.exact() {
		// regexp2-only: route-B 提到必需字面量者改为字面量门控, 否则仍 always-on.
		if len(cp.literals) > 0 {
			return "regexp2-gated"
		}
		return "regexp2-always-on"
	}
	if len(cp.literals) == 0 {
		return "always-on"
	}
	return "primary"
}
