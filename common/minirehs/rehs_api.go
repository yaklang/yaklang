package minirehs

import (
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

// 本文件是 minirehs 面向 yak 语言的高层封装 (Exports). 设计目标: 让 yak 用一行
//
//	group = rehs.BuildGroup(regexprs)~
//	group.Match(data)
//
// 完成"成百上千条正则统一编译, 一次扫描判定哪些命中"的 Hyperscan 式批量匹配。
//
// 后端策略 (默认 CGO 最强 + 按系统逐步退化, 见 selectBackend / mvs_cgo.go vs mvs_stub.go):
//   - 默认走自托管 mvscan (BackendMVS): 启用 CGO 时自动编入纯 C99 位并行内核 (最强档);
//     无 CGO (CGO_ENABLED=0) 时优雅退化为纯 Go 参考执行器, 全平台可移植、功能一致。
//   - 全程零外部依赖、不加载任何动态库 (不依赖 libhs/vectorscan); 这是本引擎自托管、可移植、
//     "分发不崩溃"的核心定位。
//
// 关键词: rehs, BuildGroup, 多正则批量匹配, compile then scan, Exports, yak

// Group 是一组正则的统一编译产物 (不可变、并发安全只读), 面向 yak 的高层句柄。
type Group struct {
	db    Database
	exprs []string
	pool  sync.Pool
}

// GroupMatch 是一次命中, 字段对 yak 友好。
type GroupMatch struct {
	Index   int    // 命中的正则在 BuildGroup 入参中的下标
	Pattern string // 命中的正则表达式
	From    int    // 命中起始字节偏移; 存在性模式或不可定位 (regexp2 兜底) 时为 -1
	To      int    // 命中结束字节偏移; 同上为 -1
	Value   string // data[From:To]; 不可定位时为空串
}

// groupConfig 由 GroupOption 构造。
type groupConfig struct {
	caseInsensitive bool
	dotAll          bool
	multiline       bool
	existenceOnly   bool        // true 则只判存在性 (Match/MatchedIndexes 最快), Find 偏移为 -1
	backend         BackendKind // 默认 BackendMVS; 可显式覆盖
	minLiteralLen   int
}

// GroupOption 配置 BuildGroup。
type GroupOption func(*groupConfig)

// WithGroupCaseInsensitive 大小写不敏感 (等价对每条正则加 (?i)); yak 侧见 rehs.caseInsensitive()。
func WithGroupCaseInsensitive(b bool) GroupOption {
	return func(c *groupConfig) { c.caseInsensitive = b }
}

// WithGroupDotAll 让 . 匹配换行 (等价 (?s))。
func WithGroupDotAll(b bool) GroupOption {
	return func(c *groupConfig) { c.dotAll = b }
}

// WithGroupMultiline 让 ^ $ 匹配行首行尾 (等价 (?m))。
func WithGroupMultiline(b bool) GroupOption {
	return func(c *groupConfig) { c.multiline = b }
}

// WithGroupExistenceOnly 只判"哪些规则命中"而不取精确字节偏移, 走纯位运算快路径换取更高吞吐
// (适合打标/分流等只需存在性的场景)。此时 Find 的 From/To 上报 -1。
func WithGroupExistenceOnly(b bool) GroupOption {
	return func(c *groupConfig) { c.existenceOnly = b }
}

// WithGroupBackend 显式指定后端: "mvs"(默认) / "engine" / "stdlib"。
// 选不到/不可用时回退到 mvscan, 绝不因环境缺失而失败。
func WithGroupBackend(name string) GroupOption {
	return func(c *groupConfig) { c.backend = backendFromName(name) }
}

// WithGroupMinLiteralLen 设定必需字面量最小长度阈值 (影响预过滤精度)。
func WithGroupMinLiteralLen(n int) GroupOption {
	return func(c *groupConfig) { c.minLiteralLen = n }
}

func backendFromName(name string) BackendKind {
	switch name {
	case "engine", "re2":
		return BackendEngine
	case "stdlib", "regexp":
		return BackendStdlib
	case "mvs", "mvscan", "":
		return BackendMVS
	default:
		return BackendMVS
	}
}

// BuildGroup 把一组正则统一编译为可复用、可并发的 Group（导出名为 rehs.BuildGroup）
//
// 借鉴 Hyperscan「compile then scan」模型：成百上千条正则一次编译、一次扫描判定哪些命中，
// 避免逐条全量匹配的 O(N_patterns × N_bytes) 开销。典型场景包括 MITM 流量打标、IOC 批量匹配、
// 指纹识别规则集。编译后的 Group 不可变、并发安全只读；用毕须 group.Close() 释放资源。
//
// 参数:
//   - patterns: 正则表达式列表，接受 yak 字符串列表，也容忍 []string / []interface{} / 单字符串
//   - opts: 可选配置，如 rehs.existenceOnly()、rehs.caseInsensitive() 等，见 GroupOption 选项表
//
// 返回值:
//   - group: 编译成功的 Group 句柄，提供 Match / Find / Scan 等方法
//   - err: patterns 为空，或任一正则 RE2/regexp2 均无法编译时返回错误
//
// <|EXAMPLE_START|> 基础用法：编译并判定命中
// ```
// group = rehs.BuildGroup(["admin", "(?i)password", "token=\\w+"])~
// data = "see admin token=abc123"
// assert group.Match(data), "should match admin or token"
// group.Close()
// ```
// <|EXAMPLE_END|>
//
// <|EXAMPLE_START|> 取全部命中（含偏移与内容）
// ```
// group = rehs.BuildGroup(["admin", "token=\\w+"])~
//
//	for m in group.Find("admin token=xyz") {
//	    println(m.Index, m.Pattern, m.From, m.To, m.Value)
//	}
//
// group.Close()
// ```
// <|EXAMPLE_END|>
//
// <|EXAMPLE_START|> 存在性快路径（更高吞吐）
// ```
// group = rehs.BuildGroup(["secret", "[0-9]{6}"], rehs.existenceOnly())~
// assert group.Match("the otp is 123456"), "existence mode should hit"
// group.Close()
// ```
// <|EXAMPLE_END|>
func BuildGroup(patterns interface{}, opts ...GroupOption) (*Group, error) {
	exprs := utils.InterfaceToStringSlice(patterns)
	if len(exprs) == 0 {
		return nil, utils.Error("rehs.BuildGroup: empty pattern list")
	}

	gc := &groupConfig{backend: BackendMVS, minLiteralLen: 2}
	for _, o := range opts {
		if o != nil {
			o(gc)
		}
	}

	var flags Flag
	if gc.caseInsensitive {
		flags |= FlagCaseless
	}
	if gc.dotAll {
		flags |= FlagDotAll
	}
	if gc.multiline {
		flags |= FlagMultiline
	}

	ps := make([]Pattern, 0, len(exprs))
	for i, e := range exprs {
		ps = append(ps, Pattern{ID: PatternID(i), Expr: e, Flags: flags})
	}

	db, err := Compile(ps,
		WithBackend(gc.backend),
		WithReportLocation(!gc.existenceOnly),
		WithMinLiteralLen(gc.minLiteralLen),
	)
	if err != nil {
		return nil, err
	}

	g := &Group{db: db, exprs: append([]string(nil), exprs...)}
	g.pool.New = func() interface{} {
		sc, _ := g.db.NewScratch()
		return sc
	}
	return g, nil
}

func (g *Group) getScratch() Scratch {
	if s, ok := g.pool.Get().(Scratch); ok && s != nil {
		return s
	}
	s, _ := g.db.NewScratch()
	return s
}

func (g *Group) putScratch(s Scratch) {
	if s != nil {
		g.pool.Put(s)
	}
}

func (g *Group) toGroupMatch(m Match, data []byte) *GroupMatch {
	gm := &GroupMatch{Index: int(m.ID), From: m.From, To: m.To}
	if int(m.ID) >= 0 && int(m.ID) < len(g.exprs) {
		gm.Pattern = g.exprs[m.ID]
	}
	if m.From >= 0 && m.To >= m.From && m.To <= len(data) {
		gm.Value = string(data[m.From:m.To])
	} else {
		gm.From, gm.To = -1, -1
	}
	return gm
}

// Match 判定 data 中是否有任意一条正则命中（存在性，命中即停，最快；导出名为 group.Match）
//
// 参数:
//   - data: 待扫描数据，接受 string / []byte / 任意可转字节
//
// 返回值:
//   - 是否至少命中一条正则
func (g *Group) Match(data interface{}) bool {
	b := utils.InterfaceToBytes(data)
	sc := g.getScratch()
	defer g.putScratch(sc)
	found := false
	_ = g.db.Scan(b, sc, func(m Match) bool {
		found = true
		return false // 命中即停
	})
	return found
}

// MatchString 是 Match 的字符串便捷封装（导出名为 group.MatchString）。
func (g *Group) MatchString(s string) bool { return g.Match(s) }

// MatchBytes 是 Match 的字节切片便捷封装（导出名为 group.MatchBytes）。
func (g *Group) MatchBytes(b []byte) bool { return g.Match(b) }

// Find 返回 data 中所有命中（导出名为 group.Find）
//
// 每条命中含下标、正则、字节偏移与命中内容。定位模式下给出精确 [From, To)；
// 存在性模式 (rehs.existenceOnly) 或 regexp2 兜底正则的 From/To 为 -1。
//
// 参数:
//   - data: 待扫描数据，接受 string / []byte / 任意可转字节
//
// 返回值:
//   - 命中列表，元素字段：Index / Pattern / From / To / Value
func (g *Group) Find(data interface{}) []*GroupMatch {
	b := utils.InterfaceToBytes(data)
	sc := g.getScratch()
	defer g.putScratch(sc)
	var out []*GroupMatch
	_ = g.db.Scan(b, sc, func(m Match) bool {
		out = append(out, g.toGroupMatch(m, b))
		return true
	})
	return out
}

// MatchedIndexes 返回命中的正则下标集合（按首次命中序去重；导出名为 group.MatchedIndexes）
//
// 参数:
//   - data: 待扫描数据
//
// 返回值:
//   - 命中的正则下标列表
func (g *Group) MatchedIndexes(data interface{}) []int {
	b := utils.InterfaceToBytes(data)
	sc := g.getScratch()
	defer g.putScratch(sc)
	seen := make(map[int]struct{})
	var out []int
	_ = g.db.Scan(b, sc, func(m Match) bool {
		id := int(m.ID)
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			out = append(out, id)
		}
		return true
	})
	return out
}

// MatchedPatterns 返回命中的正则表达式集合（按首次命中序去重；导出名为 group.MatchedPatterns）
//
// 参数:
//   - data: 待扫描数据
//
// 返回值:
//   - 命中的正则表达式列表
func (g *Group) MatchedPatterns(data interface{}) []string {
	idxs := g.MatchedIndexes(data)
	out := make([]string, 0, len(idxs))
	for _, i := range idxs {
		if i >= 0 && i < len(g.exprs) {
			out = append(out, g.exprs[i])
		}
	}
	return out
}

// Count 返回 data 中命中的总次数（含同一正则的多次命中；导出名为 group.Count）
//
// 参数:
//   - data: 待扫描数据
//
// 返回值:
//   - 命中总次数
func (g *Group) Count(data interface{}) int {
	b := utils.InterfaceToBytes(data)
	sc := g.getScratch()
	defer g.putScratch(sc)
	n := 0
	_ = g.db.Scan(b, sc, func(m Match) bool {
		n++
		return true
	})
	return n
}

// Scan 流式扫描 data，每命中一次回调 cb（导出名为 group.Scan）
//
// cb 返回 false 时提前终止扫描；返回 true 继续收集后续命中。
//
// 参数:
//   - data: 待扫描数据
//   - cb: 命中回调，参数为 *GroupMatch
func (g *Group) Scan(data interface{}, cb func(*GroupMatch) bool) {
	b := utils.InterfaceToBytes(data)
	sc := g.getScratch()
	defer g.putScratch(sc)
	_ = g.db.Scan(b, sc, func(m Match) bool {
		if cb == nil {
			return true
		}
		return cb(g.toGroupMatch(m, b))
	})
}

// Patterns 返回本组的全部正则表达式（按编译入参序；导出名为 group.Patterns）。
func (g *Group) Patterns() []string { return append([]string(nil), g.exprs...) }

// Len 返回本组正则条数（导出名为 group.Len）。
func (g *Group) Len() int { return len(g.exprs) }

// Info 返回后端元信息（导出名为 group.Info）
//
// 字段含后端名、层级、是否 SIMD、总条数、always-on 条数等，便于确认是否启用了最强档。
//
// 返回值:
//   - DatabaseInfo 结构体
func (g *Group) Info() DatabaseInfo { return g.db.Info() }

// Close 释放后端持有的本地资源（导出名为 group.Close；纯 Go 后端为 no-op）。
//
// 返回值:
//   - 释放失败时的错误信息
func (g *Group) Close() error { return g.db.Close() }

// MatchAny 一次性判定 data 是否命中 patterns 中任意一条正则（导出名为 rehs.MatchAny）
//
// 内部临时以 existenceOnly 模式编译 Group、扫描后立即释放，不复用编译结果。
// 适合偶尔一次的快速判定；需对同一组规则反复扫描时请用 rehs.BuildGroup 编译后复用。
//
// 参数:
//   - patterns: 正则表达式列表（同 BuildGroup 的 patterns）
//   - data: 待扫描数据，接受 string / []byte / 任意可转字节
//
// 返回值:
//   - ok: 是否至少命中一条正则
//   - err: patterns 无效或编译失败时返回错误
//
// <|EXAMPLE_START|> 一次性存在性判定
// ```
// ok = rehs.MatchAny(["\\bfoo\\b", "bar"], "a foo b")~
// assert ok, "should hit word foo"
//
// ok2 = rehs.MatchAny(["zzz"], "nothing")~
// assert ok2 == false, "should not hit"
// ```
// <|EXAMPLE_END|>
func MatchAny(patterns interface{}, data interface{}) (bool, error) {
	g, err := BuildGroup(patterns, WithGroupExistenceOnly(true))
	if err != nil {
		return false, err
	}
	defer g.Close()
	return g.Match(data), nil
}

// rehsCaseInsensitive 启用大小写不敏感匹配（导出名为 rehs.caseInsensitive）
//
// 等价于对每条正则添加 (?i) 前缀。作为 rehs.BuildGroup 的可选参数传入。
//
// 返回值:
//   - 供 BuildGroup 使用的 GroupOption
//
// Example:
// ```
// opt = rehs.caseInsensitive()
// ```
func rehsCaseInsensitive() GroupOption { return WithGroupCaseInsensitive(true) }

// rehsDotAll 让 . 匹配换行符（导出名为 rehs.dotAll）
//
// 等价于对每条正则添加 (?s) 前缀。作为 rehs.BuildGroup 的可选参数传入。
//
// 返回值:
//   - 供 BuildGroup 使用的 GroupOption
//
// Example:
// ```
// opt = rehs.dotAll()
// ```
func rehsDotAll() GroupOption { return WithGroupDotAll(true) }

// rehsMultiline 让 ^ $ 匹配行首行尾（导出名为 rehs.multiline）
//
// 等价于对每条正则添加 (?m) 前缀。作为 rehs.BuildGroup 的可选参数传入。
//
// 返回值:
//   - 供 BuildGroup 使用的 GroupOption
//
// Example:
// ```
// opt = rehs.multiline()
// ```
func rehsMultiline() GroupOption { return WithGroupMultiline(true) }

// rehsExistenceOnly 只判「哪些规则命中」而不取精确字节偏移（导出名为 rehs.existenceOnly）
//
// 走纯位运算存在性快路径，吞吐更高，适合打标/分流等只需存在性的场景。
// 此时 Find 的 From/To 上报 -1。作为 rehs.BuildGroup 的可选参数传入。
//
// 返回值:
//   - 供 BuildGroup 使用的 GroupOption
//
// Example:
// ```
// opt = rehs.existenceOnly()
// ```
func rehsExistenceOnly() GroupOption { return WithGroupExistenceOnly(true) }

// rehsMinLiteralLen 设定必需字面量最小长度阈值（导出名为 rehs.minLiteralLen）
//
// 影响预过滤精度与 always-on 归类。作为 rehs.BuildGroup 的可选参数传入。
//
// 参数:
//   - n: 字面量最小长度，默认 2
//
// 返回值:
//   - 供 BuildGroup 使用的 GroupOption
//
// Example:
// ```
// opt = rehs.minLiteralLen(3)
// ```
func rehsMinLiteralLen(n int) GroupOption { return WithGroupMinLiteralLen(n) }

// rehsBackend 显式指定匹配后端（导出名为 rehs.backend）
//
// name 可选 "mvs"（默认，自托管 mvscan）/ "engine"（Teddy+RE2）/ "stdlib"（标准库逐条匹配，作基线）。
// 选不到或不可用时自动回退到 mvs，不会因环境缺失而失败。
//
// 参数:
//   - name: 后端名称
//
// 返回值:
//   - 供 BuildGroup 使用的 GroupOption
//
// Example:
// ```
// opt = rehs.backend("mvs")
// ```
func rehsBackend(name string) GroupOption { return WithGroupBackend(name) }

// Exports 是 minirehs 面向 yak 的导出表, 在脚本引擎中以 rehs 名注册。
var Exports = map[string]interface{}{
	"BuildGroup": BuildGroup,
	"MatchAny":   MatchAny,

	// BuildGroup 选项 (yak 风格: rehs.caseInsensitive() ...)
	"caseInsensitive": rehsCaseInsensitive,
	"dotAll":          rehsDotAll,
	"multiline":       rehsMultiline,
	"existenceOnly":   rehsExistenceOnly,
	"minLiteralLen":   rehsMinLiteralLen,
	"backend":         rehsBackend,
}
