package fp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	regexp "github.com/VillanCh/go-pcre2-lite/regexp2"
	"golang.org/x/sync/singleflight"

	"github.com/yaklang/yaklang/common/fp/imagecache"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minirehs"
	utils2 "github.com/yaklang/yaklang/common/utils"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
)

// nmapPrefilter compiles every match regex on a probe once.
// minirehs reports which rules can hit. pcre2-lite then confirms the hit and
// extracts the capture groups nmap substitutions need. Rules minirehs cannot
// compile stay in the candidate list, so a miss there is not a skipped match.
type nmapPrefilter struct {
	db          minirehs.Database
	compiled    []int
	force       []int
	fallbackAll bool
}

type nmapPrefilterLog struct{}

func (nmapPrefilterLog) Infof(string, ...interface{})  {}
func (nmapPrefilterLog) Debugf(string, ...interface{}) {}
func (nmapPrefilterLog) Warnf(format string, args ...interface{}) {
	log.Warnf(format, args...)
}
func (nmapPrefilterLog) Errorf(format string, args ...interface{}) {
	log.Errorf(format, args...)
}

// scanBanner 把横幅变成和 pcre2 相同的“一字节一个码点”视图。
// minirehs 按 UTF-8 解码: 0x80 会变成 RuneError, 而 nmap 的 \x80 要匹配码点 U+0080。
// ASCII 原样扫描。出现高位字节时, 把每个字节展开成对应码点的 UTF-8, 只用于存在性判断。
func scanBanner(banner []byte) []byte {
	for _, b := range banner {
		if b < 0x80 {
			continue
		}
		out := make([]byte, 0, len(banner)+len(banner)/4)
		for _, item := range banner {
			if item < 0x80 {
				out = append(out, item)
				continue
			}
			out = append(out, 0xC0|item>>6, 0x80|item&0x3F)
		}
		return out
	}
	return banner
}

func nmapPattern(rule *NmapMatch) (string, minirehs.Flag, bool) {
	if rule == nil || rule.MatchRule == nil {
		return "", 0, false
	}
	lazy, ok := rule.MatchRule.(*lazyRegexp)
	if !ok || lazy == nil || lazy.raw == "" {
		return "", 0, false
	}
	var flags minirehs.Flag
	if lazy.options&regexp.IgnoreCase != 0 {
		flags |= minirehs.FlagCaseless
	}
	if lazy.options&regexp.Singleline != 0 {
		flags |= minirehs.FlagDotAll
	}
	expr := lazy.raw
	if flags != 0 {
		prefix := "(?"
		if flags&minirehs.FlagCaseless != 0 {
			prefix += "i"
		}
		if flags&minirehs.FlagDotAll != 0 {
			prefix += "s"
		}
		expr = prefix + ")" + lazy.raw
	}
	if !regexp_utils.NewYakRegexpUtils(expr).CanUse() {
		return "", 0, false
	}
	return lazy.raw, flags, true
}

func compileNmapPrefilter(rules []*NmapMatch) *nmapPrefilter {
	nmapPrefilterCompileCount.Add(1)
	filter := &nmapPrefilter{}
	patterns := make([]minirehs.Pattern, 0, len(rules))
	for i, rule := range rules {
		raw, flags, ok := nmapPattern(rule)
		if !ok {
			filter.force = append(filter.force, i)
			continue
		}
		patterns = append(patterns, minirehs.Pattern{
			ID:    minirehs.PatternID(len(filter.compiled)),
			Expr:  raw,
			Flags: flags,
		})
		filter.compiled = append(filter.compiled, i)
	}
	if len(patterns) == 0 {
		return filter
	}
	// 规则顺序和选项不变时，镜像放在本机缓存里。下次进程直接加载，不再解析这批正则。
	db, err := imagecache.Open("service", patterns,
		minirehs.WithBackend(minirehs.BackendMVS),
		minirehs.WithReportLocation(false),
		minirehs.WithLogger(nmapPrefilterLog{}),
	)
	if err != nil {
		log.Warnf("service match prefilter fell back to per-rule match: %v", err)
		filter.fallbackAll = true
		filter.compiled = nil
		return filter
	}
	filter.db = db
	return filter
}

var (
	nmapPrefilterGlobal       sync.Map
	nmapPrefilterFlight       singleflight.Group
	nmapPrefilterCount        atomic.Int32
	nmapPrefilterCompileCount atomic.Int32
	// onServicePrefilterCompile 只在真正编译时回调，测试用来确认同一探针只编译一次。
	onServicePrefilterCompile atomic.Value
)

const maxServicePrefilterDB = 2048

type nmapPrefilterGlobalKey struct {
	probe *NmapProbe
	rules int
}

func prefilterForBlock(block *RuleBlock) *nmapPrefilter {
	if block == nil {
		return nil
	}
	if block.Probe == nil {
		return compileNmapPrefilter(block.Matched)
	}
	key := fmt.Sprintf("%p/%d", block.Probe, len(block.Matched))
	v, _, _ := nmapPrefilterFlight.Do(key, func() (interface{}, error) {
		mapKey := nmapPrefilterGlobalKey{probe: block.Probe, rules: len(block.Matched)}
		if cached, ok := nmapPrefilterGlobal.Load(mapKey); ok {
			return cached.(*nmapPrefilter), nil
		}
		start := time.Now()
		compiled := compileNmapPrefilter(block.Matched)
		if hook, _ := onServicePrefilterCompile.Load().(func(*NmapProbe)); hook != nil {
			hook(block.Probe)
		}
		if nmapPrefilterCount.Load() < maxServicePrefilterDB {
			if _, loaded := nmapPrefilterGlobal.LoadOrStore(mapKey, compiled); !loaded {
				nmapPrefilterCount.Add(1)
			}
		}
		if len(block.Matched) >= 64 {
			log.Debugf("service match prefilter ready: probe=%s rules=%d cost=%s", block.Probe.Name, len(block.Matched), time.Since(start))
		}
		return compiled, nil
	})
	filter, _ := v.(*nmapPrefilter)
	if filter == nil {
		return compileNmapPrefilter(block.Matched)
	}
	return filter
}

func warmServicePrefilters(empty *RuleBlock, blocks []*RuleBlock) {
	warmOne := func(block *RuleBlock) {
		if block == nil || block.Probe == nil || len(block.Matched) < 64 {
			return
		}
		go block.prefilterOnce.Do(func() {
			block.prefilter = prefilterForBlock(block)
		})
	}
	warmOne(empty)
	for _, block := range blocks {
		warmOne(block)
	}
}

func (b *RuleBlock) candidateMatches(banner []byte) []*NmapMatch {
	if b == nil || len(b.Matched) == 0 {
		return nil
	}
	b.prefilterOnce.Do(func() {
		b.prefilter = prefilterForBlock(b)
	})
	if b.prefilter == nil || b.prefilter.fallbackAll || b.prefilter.db == nil && len(b.prefilter.force) != len(b.Matched) {
		return b.Matched
	}
	return b.prefilter.selectMatches(b.Matched, banner)
}

func (p *nmapPrefilter) selectMatches(rules []*NmapMatch, banner []byte) []*NmapMatch {
	if p == nil || p.fallbackAll || p.db == nil {
		return rules
	}
	hit := make([]bool, len(rules))
	for _, idx := range p.force {
		if idx >= 0 && idx < len(hit) {
			hit[idx] = true
		}
	}
	scratch, err := p.db.NewScratch()
	if err != nil {
		return rules
	}
	if err := p.db.Scan(scanBanner(banner), scratch, func(m minirehs.Match) bool {
		id := int(m.ID)
		if id >= 0 && id < len(p.compiled) {
			idx := p.compiled[id]
			if idx >= 0 && idx < len(hit) {
				hit[idx] = true
			}
		}
		return true
	}); err != nil {
		return rules
	}
	selected := make([]*NmapMatch, 0, len(rules))
	for i, rule := range rules {
		if hit[i] {
			selected = append(selected, rule)
		}
	}
	return selected
}

func applyMatchRules(ctx context.Context, block *RuleBlock, banner []byte, port int, host net.IP, safeBanner string, proto TransportProto) *FingerprintInfo {
	if block == nil || len(banner) == 0 {
		return nil
	}
	runes := utils2.AsciiBytesToRegexpMatchedRunes(banner)
	for _, rule := range block.candidateMatches(banner) {
		if ctx != nil && ctx.Err() != nil {
			return nil
		}
		if info := match(rule, runes, port, host, safeBanner, proto); info != nil {
			return info
		}
	}
	return nil
}
