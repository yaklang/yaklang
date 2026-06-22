// Package minirehs 实现一个可移植的多正则批量匹配引擎,借鉴 Intel Hyperscan 的
// "统一编译, 一次扫描" (compile then scan) 模型: 把成百上千条正则统一编译为一个
// 不可变的 Database, 对输入数据只扫描一次即可返回所有命中, 从而避免"几百条正则
// 逐条匹配"造成的 O(N_patterns x N_bytes) 性能问题.
//
// 设计第一约束是可移植: 默认构建 (CGO_ENABLED=0, 不加任何 build tag) 必须在所有
// 平台/架构上编译运行且功能完整, 走纯 Go 自研引擎 (字面量预过滤 Aho-Corasick +
// RE2 验证). 当启用 CGO 且带 minirehs_cgo tag 时, 字面量预过滤这一步会切换为
// 自带的 SIMD (Teddy/AC 加速) 实现, 不依赖任何外部 C 库, 缺失时优雅退化为纯 Go.
//
// 重要语义说明: 本引擎与 Go 标准库 regexp 一样是 RE2 自动机方法, 不支持
// backreference 与任意 lookaround. 这是数学本质决定的, 不是实现缺陷.
//
// 关键词: minirehs, multi-regex, compile then scan, prefilter, RE2
package minirehs

// PatternID 是调用方为每条正则指定的稳定标识, 命中结果用它回指.
type PatternID uint32

// Flag 控制单条 pattern 的匹配语义, 语义对齐 Hyperscan flags 子集中可在 RE2 表达的部分.
type Flag uint32

const (
	// FlagCaseless 大小写不敏感, 等价于在正则前加 (?i).
	FlagCaseless Flag = 1 << iota
	// FlagDotAll 让 . 匹配换行, 等价于 (?s).
	FlagDotAll
	// FlagMultiline 让 ^ $ 匹配行首行尾, 等价于 (?m).
	FlagMultiline
)

// UnsupportedPolicy 决定遇到本引擎 (RE2 自动机) 不支持的构造时如何处理.
type UnsupportedPolicy uint8

const (
	// DefaultPolicy 表示该条 pattern 未显式指定策略, 使用全局默认 (见 WithDefaultUnsupportedPolicy).
	DefaultPolicy UnsupportedPolicy = iota
	// Reject 报错, 拒绝整体编译, 并指出哪条 pattern 的哪个构造不被支持.
	Reject
	// Fallback 将该条 pattern 降级到 stdlib 子引擎 (Composite). 注意: stdlib 同样是
	// RE2, 对 backref/lookaround 也不支持, 此时 Fallback 会再次失败并回退为 Reject.
	Fallback
)

// BackendKind 标识实际选用的后端.
type BackendKind uint8

const (
	// Auto 在编译入口按可用性自动探测最优后端 (当前: 自研引擎, 保证精确偏移与可移植).
	Auto BackendKind = iota
	// BackendEngine 是自研多正则引擎 (字面量预过滤 + RE2 验证), 始终可用, RE2 精确偏移语义.
	BackendEngine
	// BackendStdlib 是 stdlib regexp 逐条匹配后端, 既作正确性兜底, 也用作基线/oracle.
	BackendStdlib
	// BackendVectorscan 是基于 Vectorscan/Hyperscan 的高性能"存在性"匹配后端 (可选加速):
	// 把所有正则编译进单一 SIMD 自动机, 一次扫描判定"哪些规则命中", 适合 MITM 打标等
	// 以命中存在性为准的场景 (命中以 From/To=-1 上报, 与 regexp2-only 语义一致)。
	// 仅在 -tags minirehs_vectorscan 构建且运行时能加载到 libhs 时可用; 否则优雅退化为引擎。
	BackendVectorscan
	// BackendMVS 是自托管 mvscan 引擎 (字节级 Glushkov 位并行 NFA + 字面量预过滤) 的"存在性"
	// 匹配后端: 把每条正则编译为字节级位置自动机, 一次扫描判定"哪些规则命中" (From/To=-1)。
	// 当前为纯 Go 参考实现, 始终可用; 后续以 build tag 接入 SIMD/CGO 加速档并以本实现做差分对照。
	BackendMVS
)

func (b BackendKind) String() string {
	switch b {
	case Auto:
		return "auto"
	case BackendEngine:
		return "engine"
	case BackendStdlib:
		return "stdlib"
	case BackendVectorscan:
		return "vectorscan"
	case BackendMVS:
		return "mvs"
	default:
		return "unknown"
	}
}

// Pattern 是一条待编译的正则.
type Pattern struct {
	ID            PatternID
	Expr          string
	Flags         Flag
	OnUnsupported UnsupportedPolicy // 零值为 DefaultPolicy, 即采用全局默认策略 (默认 Reject)
}

// Match 是一次命中. 语义对齐 stdlib regexp 的 FindAllIndex: [From, To) 为命中字节区间.
type Match struct {
	ID   PatternID
	From int
	To   int
}

// MatchHandler 是命中回调. 返回 false 表示停止本次扫描.
type MatchHandler func(m Match) (cont bool)

// PatternReport 记录单条 pattern 在编译期的处置结果, 便于排障与性能分析.
type PatternReport struct {
	ID          PatternID
	Disposition string // "primary" | "always-on" | "fallback-stdlib" | "rejected"
	Reason      string // 不支持/降级时的具体原因 (英文)
	HasLiteral  bool   // 是否提取到必需字面量 (影响 prefilter 收益)
}

// DatabaseInfo 返回 Database 的元信息.
type DatabaseInfo struct {
	Backend     BackendKind
	Tier        int  // 0 最快, 数字越大越慢; 本模块: 2=SIMD prefilter, 3=scalar prefilter, 4=stdlib
	SIMD        bool // prefilter 是否启用了自带 SIMD 实现
	Composite   bool // 是否含 stdlib 兜底子集
	NumPatterns int
	NumAlwaysOn int // 无必需字面量、每次扫描都要运行的 pattern 数 (性能风险提示)
	Reports     []PatternReport
}
