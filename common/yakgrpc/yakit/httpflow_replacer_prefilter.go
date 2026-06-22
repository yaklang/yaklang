package yakit

import (
	"os"
	"strings"
	"sync"

	"github.com/dlclark/regexp2"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minirehs"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
)

// 本文件用 minirehs (自托管多正则批量匹配引擎, Hyperscan 式 "compile then scan") 为 MITM 染色/提取
// 路径维护"一个统一编译的规则 Group": 把所有可被 RE2 自动机 + 必需字面量安全门控的规则一次性编译进
// 一个不可变、并发安全的引擎, 每个报文只扫描一次即可判定"哪些规则的必需字面量/骨架可能命中", 从而
// 跳过其余规则昂贵的逐条 regexp2 匹配, 把 O(N_patterns x N_bytes) 降到 ~O(N_bytes)。
//
// 关键词: MITM replacer, 统一编译, 存在性预过滤, minirehs.Compile, prefilter, 不漏报

// MITMReplacerPrefilterEnabled 是 MITM 规则统一预过滤 (维护一个 Group) 的总开关。默认开启;
// 设置环境变量 YAKIT_MITM_REPLACER_DISABLE_PREFILTER=1/true/on 可一键关闭, 退化为逐条 regexp2 的
// 旧行为, 便于 A/B 体感对比与万一出问题时紧急止血。运行期也可直接改这个包级变量。
var MITMReplacerPrefilterEnabled = func() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("YAKIT_MITM_REPLACER_DISABLE_PREFILTER"))) {
	case "1", "true", "yes", "on":
		return false
	}
	return true
}()

// mitmRulePrefilter 是用 minirehs 维护的"多规则统一编译"存在性预过滤器, 即一个常驻的规则 Group。
//
// 正确性 (绝不漏报, 这是安全工具的底线) 由构造保证:
//  1. 只把 RE2 可精确表达且含必需字面量的规则 (disposition=="primary") 纳入"可跳过集合"; 凡 regexp2-only
//     (lookaround/backref) 或无必需字面量的规则一律标记为 always-candidate (prefilterID<0) 永不跳过。
//  2. 预过滤引擎强制开启 Multiline (对齐 MITM 默认的 ECMAScript|Multiline; 否则 ^ $ 语义会更窄) +
//     DotAll (纯放宽, 只会多报候选不会漏)。即预过滤语言是规则语言的超集, 不会把真正会命中的报文判为不命中。
//  3. 命中与否最终仍由原 regexp2 在候选规则上精确判定 (染色/提取/替换语义完全不变); 预过滤只决定
//     "是否值得对该规则跑 regexp2"。
type mitmRulePrefilter struct {
	db   minirehs.Database
	pool *sync.Pool // 复用 minirehs.Scratch (非并发安全, 每次扫描独占一份)
	size int        // 候选位图尺寸 = 规则总数 (rule 下标即 PatternID)
}

// prefilterExprOf 返回某条规则用于预过滤编译的表达式。ExactMatch 规则按字面量转义 (与 Compile 一致),
// 其余直接用原始规则串 (其内联 (?i)/(?s) 前缀由 minirehs 的 RE2/regexp2 解析负责)。
func prefilterExprOf(r *MITMReplaceRule) string {
	if r == nil {
		return ""
	}
	if r.GetExactMatch() {
		return regexp2.Escape(r.Rule)
	}
	return r.Rule
}

// buildMITMRulePrefilter 用 rules (已启用规则, 切片下标即规则稳定 ID) 构造预过滤器, 并回填每条规则的
// prefilterID: >=0 表示纳入预过滤(可被存在性预筛跳过), -1 表示 always-candidate(永不跳过)。
// 返回 nil 表示没有可纳入的 primary 规则或编译失败/被禁用, 调用方应退化为"全部规则都跑"(行为与旧版一致)。
func buildMITMRulePrefilter(rules []*MITMReplaceRule) *mitmRulePrefilter {
	// 默认所有规则 always-candidate, 后面只把 primary 规则回填为可跳过。
	for _, r := range rules {
		if r != nil {
			r.prefilterID = -1
		}
	}
	if !MITMReplacerPrefilterEnabled || len(rules) == 0 {
		return nil
	}

	// 1) 逐条预筛 minirehs 可编译性, 构造探针 patterns (PatternID = 规则下标)。
	// minirehs.Compile 默认策略遇到不可编译会拒绝整批, 故必须先筛掉不可编译者 (它们保持 always-candidate)。
	flags := minirehs.FlagMultiline | minirehs.FlagDotAll
	probes := make([]minirehs.Pattern, 0, len(rules))
	for i, r := range rules {
		if r == nil {
			continue
		}
		expr := prefilterExprOf(r)
		if expr == "" {
			continue
		}
		if !regexp_utils.NewYakRegexpUtils(expr).CanUse() {
			continue // RE2/regexp2 都编译不了 -> 永不跳过
		}
		probes = append(probes, minirehs.Pattern{
			ID:    minirehs.PatternID(i),
			Expr:  expr,
			Flags: flags,
		})
	}
	if len(probes) == 0 {
		return nil
	}

	// 2) 探针编译一次, 读取每条 disposition, 仅保留 primary (RE2-exact + 必需字面量) 进入可跳过集合。
	// regexp2-gated / *-always-on 不纳入: 它们的 regexp2 语义与 MITM 的 ECMAScript 可能有细微差异,
	// 为保证不漏报一律 always-candidate; 也避免它们的 always-on 扫描在每个报文上做无谓开销。
	probeDB, err := minirehs.Compile(probes,
		minirehs.WithReportLocation(false),
		minirehs.WithBackend(minirehs.BackendMVS),
	)
	if err != nil {
		log.Warnf("mitm rule prefilter probe compile failed, fallback to per-rule regexp2: %v", err)
		return nil
	}
	primaryIDs := make(map[int]bool)
	for _, rep := range probeDB.Info().Reports {
		if rep.Disposition == "primary" {
			primaryIDs[int(rep.ID)] = true
		}
	}
	_ = probeDB.Close()
	if len(primaryIDs) == 0 {
		log.Debugf("mitm rule prefilter: no primary (RE2 + required-literal) rules, prefilter disabled")
		return nil
	}

	// 3) 用 primary 子集编译真正用于扫描的 db (ID 仍是规则下标, 命中可直接回指)。
	ps := make([]minirehs.Pattern, 0, len(primaryIDs))
	for _, p := range probes {
		if primaryIDs[int(p.ID)] {
			ps = append(ps, p)
		}
	}
	db, err := minirehs.Compile(ps,
		minirehs.WithReportLocation(false),
		minirehs.WithBackend(minirehs.BackendMVS),
	)
	if err != nil {
		log.Warnf("mitm rule prefilter compile failed, fallback to per-rule regexp2: %v", err)
		return nil
	}

	// 回填 primary 规则的 prefilterID = 自身下标 (纳入预过滤, 可被跳过)。
	for i, r := range rules {
		if r != nil && primaryIDs[i] {
			r.prefilterID = i
		}
	}

	pf := &mitmRulePrefilter{db: db, size: len(rules)}
	pf.pool = &sync.Pool{New: func() interface{} {
		sc, _ := db.NewScratch()
		return sc
	}}
	info := db.Info()
	log.Infof("mitm rule prefilter built: %d/%d rules gated by unified minirehs group, backend=%s simd=%v alwaysOn=%d",
		len(ps), len(rules), info.Backend.String(), info.SIMD, info.NumAlwaysOn)
	return pf
}

func (p *mitmRulePrefilter) getScratch() minirehs.Scratch {
	if sc, ok := p.pool.Get().(minirehs.Scratch); ok && sc != nil {
		return sc
	}
	sc, _ := p.db.NewScratch()
	return sc
}

func (p *mitmRulePrefilter) putScratch(sc minirehs.Scratch) {
	if sc != nil {
		p.pool.Put(sc)
	}
}

// fillFromInfo 对一个已切分(含 dechunk/ungzip)的报文做一次扫描, 把命中的 primary 规则下标在 mask 中置为
// 候选。为与 MatchByPacketInfo 实际扫描的字节完全对齐(避免漏报), 这里覆盖该报文在各作用域下会被匹配的
// 全部视图: Raw(整包) / HeaderRaw(去掉传输/内容编码头后的头) / BodyRaw(已解码 body) / RequestURI。
// 任一视图出现某规则的必需字面量即把它标为候选, 故是规则匹配集合的超集 (sound)。
func (p *mitmRulePrefilter) fillFromInfo(mask []bool, info *PacketInfo) {
	if p == nil || info == nil || len(mask) == 0 {
		return
	}
	sc := p.getScratch()
	defer p.putScratch(sc)
	collect := func(m minirehs.Match) bool {
		id := int(m.ID)
		if id >= 0 && id < len(mask) {
			mask[id] = true
		}
		return true // 收集本视图全部命中
	}
	scan := func(b []byte) {
		if len(b) == 0 {
			return
		}
		_ = p.db.Scan(b, sc, collect)
	}
	scan(info.Raw)
	if info.HeaderRaw != "" {
		scan([]byte(info.HeaderRaw))
	}
	scan(info.BodyRaw)
	if info.RequestURI != "" {
		scan([]byte(info.RequestURI))
	}
}

// fillFromRaw 对一段原始字节(无切分语义, 如 WebSocket 整帧)做一次扫描并标候选, 与对应匹配路径
// (MatchRawSimple 整帧 MatchString) 的字节完全一致, 故 sound。
func (p *mitmRulePrefilter) fillFromRaw(mask []bool, raw []byte) {
	if p == nil || len(raw) == 0 || len(mask) == 0 {
		return
	}
	sc := p.getScratch()
	defer p.putScratch(sc)
	_ = p.db.Scan(raw, sc, func(m minirehs.Match) bool {
		id := int(m.ID)
		if id >= 0 && id < len(mask) {
			mask[id] = true
		}
		return true
	})
}

// newCandidateMask 申请一张候选位图 (长度 = 规则总数)。
func (p *mitmRulePrefilter) newCandidateMask() []bool {
	if p == nil {
		return nil
	}
	return make([]bool, p.size)
}

// Close 释放后端持有的本地资源 (纯 Go 后端为 no-op)。
func (p *mitmRulePrefilter) Close() {
	if p != nil && p.db != nil {
		_ = p.db.Close()
	}
}
