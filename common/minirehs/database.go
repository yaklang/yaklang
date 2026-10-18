package minirehs

import (
	"errors"
	"fmt"
	"regexp"
	"regexp/syntax"
	"runtime"
	"sync"
	"sync/atomic"

	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
)

// Scratch 是每次扫描需要的可复用工作区, 用于逼近热路径低分配.
// 每个 goroutine 应独占一份 Scratch (非并发安全).
type Scratch interface {
	Close() error
}

// BatchMatchHandler 接收批扫描中的记录下标及其匹配。ScanBatch 保证按 records
// 的输入顺序串行调用 handler，因此 handler 无需承担并发同步。
type BatchMatchHandler func(record int, match Match) bool

const maxBatchLanes = 4

type batchMatch struct {
	record int
	match  Match
}

// scratch 是 Scratch 的内部实现, 持有可复用缓冲区.
type scratch struct {
	verifierText   string // Immutable subject window, shared by one record's verification worker.
	verifierTextLo int    // Absolute start of verifierText in the record.

	lower      []byte                // ASCII 小写化数据缓冲 (供大小写无关的字面量预过滤复用)
	hits       []litHit              // 字面量预过滤命中缓冲 (含位置)
	cpairs     []int32               // CGO 预过滤输出的 (end,litID) 对缓冲 (NoCGO 不使用)
	dedup      map[matchKey]struct{} // 邻域窗口验证的去重集合 (跨多次命中)
	fullDone   []bool                // 非窗口候选 pattern 是否已做过整段验证 (按 idx)
	mergedHits []int                 // mvs 合并 always-on 自动机单趟命中的成员 idx 缓冲 (复用)
	assertHits []int                 // combined scanner 的断言命中成员 idx 缓冲 (复用)

	// mvs 合并 always-on 单趟扫描的"成员级去重"缓冲 (按成员 idx 去重, 不触碰 fullDone;
	// 跨步去重由调用方用 fullDone 完成). mergedSeen 供纯 Go 路径, cseen/cmerged 供 C 内核路径.
	mergedSeen   []bool  // 纯 Go scanExist 的成员去重位图 (长度 npat)
	cseen        []byte  // C 合并 scan 的去重位图 (uint8, 长度 npat)
	cmerged      []int32 // C 返回命中成员 idx 的 int32 缓冲
	cmergedTotal int32   // combinedScan C 输出计数，置于 scratch 避免每次取局部地址逃逸
	cLocs        []int32 // C 单字 NFA 定位返回的平铺 (from,to) 对，按报文复用

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

	// 断言 always-on C 批量扫描的输出缓冲 (nfaExistsAssertMany).
	assertBatchOut    []byte
	assertBatchOutIdx []int32 // combinedScan 的 assert 命中 idx 输出缓冲

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

	// always-on merged、always-on assert 与字面量候选验证可并行执行。两个内部
	// scratch 与结果通道均由每个 Scratch 独占；短生命周期 worker 返回前必收拢。
	alwaysMergedScratch *scratch
	alwaysAssertScratch *scratch
	alwaysMergedRes     chan nativeMergedResult
	alwaysAssertRes     chan []byte

	// RE2-only 存在性模式下，把字面量命中循环中的 gated/assert 立即验证移出调用线程，
	// 与窗口及 anchored 阶段重叠；worker 使用独占 Scratch，handler 仍只在调用线程执行。
	gateTasks        []gateTask
	gateMinEnd       []int32 // minimum trigger end per localized gate, independent of event order
	gateTouched      []int32 // only these minima need clearing before the next record
	gateAsyncScratch *scratch
	gateAsyncOut     []byte
	gateAsyncRes     chan gatedResult

	anchoredAsyncScratch *scratch
	anchoredAnchorOut    []byte
	anchoredBiOut        []byte
	anchoredAsyncRes     chan anchoredResult

	// Only Group's cache uses these capacity snapshots. Updating touched
	// range owners on return accounts for historical bursts without scanning
	// every rule or adding work to Database.Scan's per-hit append loops.
	groupRangeCaps  []groupRangeCapacity
	groupRangeBytes uint64

	// ScanBatch 使用最多四个独占子 Scratch 并行处理动态记录块。结果按 lane 复用，扫描
	// 收拢后再按 record 顺序串行重放 handler，避免并发回调改变现有使用习惯。
	batchLanes   [maxBatchLanes]*scratch
	batchResults [maxBatchLanes][]batchMatch

	workerOnce      sync.Once
	workerCloseOnce sync.Once
	workerWG        sync.WaitGroup
	mergedTasks     chan alwaysMergedTask
	assertTasks     chan alwaysAssertTask
	gatedTasks      chan gatedWorkerTask
	anchoredTasks   chan anchoredWorkerTask
}

type gateTask struct {
	idx   int32
	winLo int32 // >=0: verifyGateLocalized；-1: verifyOne 等价存在性判定
}

type anchoredResult struct {
	anchor    []byte
	bi        []byte
	completed int // anchor items followed by bi items; includes a failing item
	err       error
}

type alwaysMergedTask struct {
	kernel  *mvsKernel
	data    []byte
	scratch *scratch
	result  chan<- nativeMergedResult
}

type alwaysAssertTask struct {
	kernel  *mvsKernel
	data    []byte
	idxs    []int32
	scratch *scratch
	result  chan<- []byte
}

type gatedWorkerTask struct {
	db      *mvsDB
	data    []byte
	tasks   []gateTask
	scratch *scratch
	out     []byte
	result  chan<- gatedResult
}

type gatedResult struct {
	out       []byte
	completed int // includes the failing item when err is nonnil
	err       error
}

type anchoredWorkerTask struct {
	db                   *mvsDB
	data                 []byte
	anchorBatch, biBatch []int32
	owner, scratch       *scratch
	anchorOut, biOut     []byte
	result               chan<- anchoredResult
}

func (s *scratch) Close() error {
	s.clearVerifierText()
	s.workerCloseOnce.Do(func() {
		for i := range s.batchLanes {
			if s.batchLanes[i] != nil {
				_ = s.batchLanes[i].Close()
				s.batchLanes[i] = nil
			}
		}
		if s.mergedTasks == nil {
			return
		}
		close(s.mergedTasks)
		close(s.assertTasks)
		close(s.gatedTasks)
		close(s.anchoredTasks)
		s.workerWG.Wait()
	})
	return nil
}

// Database 是编译产物, 不可变、并发安全 (只读), 可被多 goroutine 共享.
type Database interface {
	// MarshalImage exports an MVS database for LoadImage, including its Go fallback.
	// Other backends return ErrUnsupportedImage.
	MarshalImage() ([]byte, error)
	// NewScratch 分配一份与该 db 绑定的可复用扫描临时区.
	NewScratch() (Scratch, error)
	// Scan 对完整 data 做 block 扫描, 每命中一次调用 handler; handler 返回 false 提前终止.
	// data 在调用返回前保持只读；每个并行或嵌套 Scan 应使用独立 Scratch。
	// 执行失败可用 errors.As 提取 *ScanError；之前的回调不撤销，结果不可当作完整结果使用。
	Scan(data []byte, s Scratch, handler MatchHandler) error
	// ScanBatch 以 1-4 个独占 lane 并行扫描多条独立记录。并行路径先完成全部扫描，再按
	// records 输入顺序串行重放 handler；返回 false 仅停止后续回调，不取消已完成的扫描工作。
	ScanBatch(records [][]byte, s Scratch, handler BatchMatchHandler) error
	// Info 返回该 db 的元信息.
	Info() DatabaseInfo
	// Close 结束扫描生命周期并释放本地资源。调用方须先收拢所有 Scan/ScanBatch，
	// 不在 handler 内调用 Close；关闭后扫描和 NewScratch 返回 ErrClosed。
	Close() error
}

// compiledPattern 是一条经过特性 gate、字面量提取后的内部表示.
type compiledPattern struct {
	id       PatternID
	flags    Flag
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
	return compileWithConfig(patterns, cfg)
}

func compileWithConfig(patterns []Pattern, cfg *config) (Database, error) {
	if imageCompileHook != nil {
		imageCompileHook()
	}
	if len(patterns) == 0 {
		return nil, errors.New("minirehs: no patterns to compile")
	}

	var (
		supported []*compiledPattern
		reports   = make([]PatternReport, 0, len(patterns))
	)
	// Compile-only ownership: Auto/Engine/Stdlib do not retain analysis trees.
	// Each cp is unique even for duplicate IDs/expressions/flags. The map never
	// enters a published database, and the MVS loop drops entries as it consumes
	// them. Executable regexp instances are still compiled independently.
	var parsedPatterns map[*compiledPattern]*syntax.Regexp
	if cfg.backend == BackendMVS && compileASTReuseEnabled {
		parsedPatterns = make(map[*compiledPattern]*syntax.Regexp, len(patterns))
		defer func() { clear(parsedPatterns) }()
	}

	for _, p := range patterns {
		if p.Expr == "" {
			return nil, fmt.Errorf("minirehs: pattern id=%d has empty expr", p.ID)
		}

		expr := buildExprWithFlags(p)

		cp := &compiledPattern{id: p.ID, flags: p.Flags, expr: expr}

		// 尝试用标准库 RE2 编译并做字面量分析; 成功则走精确验证 + 预过滤路径.
		if re, parsed, err := compileAndParse(expr); err == nil {
			cp.v = &re2Verifier{re: re}
			if parsedPatterns != nil {
				parsedPatterns[cp] = parsed
			}
			cp.literals = extractRequiredLiterals(parsed, cfg.minLiteralLen)
			if len(cp.literals) > 0 {
				if w, ok := windowVerifiable(parsed); ok {
					cp.windowed = true
					cp.winW = w
				}
			}
		} else {
			// The successful RE2 path already validates the expression; avoid
			// compiling it a second time through YakRegexpUtils.CanUse.
			yak := regexp_utils.NewYakRegexpUtils(expr)
			if !yak.CanUse() {
				reason := "expression not compilable by RE2 nor PCRE2"
				if effectivePolicy(p.OnUnsupported, cfg.defaultPolicy) == Fallback {
					cfg.logger.Warnf("minirehs: pattern id=%d unsupported and rejected: %s", p.ID, reason)
				}
				return nil, fmt.Errorf("minirehs: pattern id=%d rejected: %s", p.ID, reason)
			}
			// RE2 不可表达 (lookaround/backref 等), 只用 VillanCh/go-pcre2-lite 验证。
			cp.v = &pcre2Verifier{yak: yak}
			// route-B: 用语言超集改写和 RE2 字面量提取必需字面量，命中后才交 PCRE2 确认。
			// 提不出则保持 always-on；健全性见 literal_routeb.go。
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
	var primary compiledDB
	if mvs, ok := backend.(*mvsBackend); ok {
		primary, err = mvs.compileParsed(supported, cfg, parsedPatterns)
	} else {
		primary, err = backend.compile(supported, cfg)
	}
	if err != nil {
		return nil, err
	}
	if m, ok := primary.(*mvsDB); ok {
		m.patternHash = imageIdentity(patterns, cfg, m.imageFlags)
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
	closed  atomic.Bool
	primary compiledDB
	info    DatabaseInfo
}

func (d *database) NewScratch() (Scratch, error) {
	if d == nil || d.closed.Load() {
		return nil, ErrClosed
	}
	return &scratch{
		lower:    make([]byte, 0, 4096),
		hits:     make([]litHit, 0, 256),
		dedup:    make(map[matchKey]struct{}, 64),
		fullDone: make([]bool, d.info.NumPatterns),
	}, nil
}

func (d *database) Scan(data []byte, s Scratch, handler MatchHandler) error {
	if d == nil || d.closed.Load() {
		return closedScanError()
	}
	sc, ok := s.(*scratch)
	if !ok || sc == nil {
		ns, err := d.NewScratch()
		if err != nil {
			return err
		}
		sc = ns.(*scratch)
		defer sc.Close()
	}
	if handler == nil {
		handler = func(Match) bool { return true }
	}
	_, err := d.primary.scan(data, sc, handler)
	return scanError(err)
}

func (d *database) ScanBatch(records [][]byte, s Scratch, handler BatchMatchHandler) error {
	if d == nil || d.closed.Load() {
		return closedScanError()
	}
	if len(records) == 0 {
		return nil
	}
	root, ok := s.(*scratch)
	if !ok || root == nil {
		ns, err := d.NewScratch()
		if err != nil {
			return err
		}
		root = ns.(*scratch)
		defer root.Close()
	}
	if handler == nil {
		handler = func(int, Match) bool { return true }
	}
	totalBytes := 0
	for _, rec := range records {
		totalBytes += len(rec)
	}
	if len(records) == 1 || totalBytes < 32*1024 || runtime.GOMAXPROCS(0) < 2 {
		for i, rec := range records {
			stop := false
			err := d.Scan(rec, root, func(m Match) bool {
				if !handler(i, m) {
					stop = true
					return false
				}
				return true
			})
			if err != nil || stop {
				return scanErrorAtRecord(err, i)
			}
		}
		return nil
	}

	lanes := runtime.GOMAXPROCS(0) / 2
	if lanes < 1 {
		lanes = 1
	} else if lanes > len(root.batchLanes) {
		lanes = len(root.batchLanes)
	}
	for lane := 0; lane < lanes; lane++ {
		if root.batchLanes[lane] == nil {
			ns, err := d.NewScratch()
			if err != nil {
				return err
			}
			root.batchLanes[lane] = ns.(*scratch)
		}
		root.batchResults[lane] = root.batchResults[lane][:0]
	}
	var wg sync.WaitGroup
	var laneErr [maxBatchLanes]error
	var laneErrRecord [maxBatchLanes]int
	var nextRecord int64
	const batchChunk = int64(8)
	wg.Add(lanes)
	for lane := 0; lane < lanes; lane++ {
		go func(lane int) {
			defer wg.Done()
			laneSc := root.batchLanes[lane]
			out := root.batchResults[lane]
			for {
				start := int(atomic.AddInt64(&nextRecord, batchChunk) - batchChunk)
				if start >= len(records) {
					break
				}
				end := start + int(batchChunk)
				if end > len(records) {
					end = len(records)
				}
				for i := start; i < end; i++ {
					err := d.Scan(records[i], laneSc, func(m Match) bool {
						out = append(out, batchMatch{record: i, match: m})
						return true
					})
					if err != nil {
						laneErr[lane] = scanErrorAtRecord(err, i)
						laneErrRecord[lane] = i
						root.batchResults[lane] = out
						return
					}
				}
			}
			root.batchResults[lane] = out
		}(lane)
	}
	wg.Wait()

	var resultPos [maxBatchLanes]int
	for {
		minLane := -1
		minRecord := 0
		nextIsError := false
		for lane := 0; lane < lanes; lane++ {
			if resultPos[lane] >= len(root.batchResults[lane]) {
				// A lane stops at its first error, so all its callbacks precede
				// this error. Merge that event by record with the other lanes.
				if laneErr[lane] != nil && (minLane < 0 || laneErrRecord[lane] < minRecord) {
					minLane, minRecord, nextIsError = lane, laneErrRecord[lane], true
				}
				continue
			}
			record := root.batchResults[lane][resultPos[lane]].record
			if minLane < 0 || record < minRecord {
				minLane = lane
				minRecord = record
				nextIsError = false
			}
		}
		if minLane < 0 {
			break
		}
		if nextIsError {
			return laneErr[minLane]
		}
		next := root.batchResults[minLane][resultPos[minLane]]
		resultPos[minLane]++
		if !handler(next.record, next.match) {
			return nil
		}
	}
	return nil
}

func (d *database) Info() DatabaseInfo { return d.info }

// Close requires callers to have joined their scans; it prevents subsequent
// scans and releases the backend once. It must not be called inside a handler.
func (d *database) Close() error {
	if d == nil || !d.closed.CompareAndSwap(false, true) {
		return nil
	}
	return d.primary.close()
}

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
	parsed, err := parseRegexSyntax(expr, syntax.Perl)
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
