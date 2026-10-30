package fingerprint

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/singleflight"

	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/fp/imagecache"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minirehs"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
)

// web 指纹里大量规则是「整页正则」。它们不是逐条线性扫过去的：
// 同一路径的正则编进一个 minirehs 库，一次扫描丢掉不可能命中的规则。
// 捕获组仍由原来的 regexp 执行，所以版本号提取不变。

type webPrefilterLog struct{}

func (webPrefilterLog) Infof(string, ...interface{})  {}
func (webPrefilterLog) Debugf(string, ...interface{}) {}
func (webPrefilterLog) Warnf(format string, args ...interface{}) {
	log.Warnf(format, args...)
}
func (webPrefilterLog) Errorf(format string, args ...interface{}) {
	log.Errorf(format, args...)
}

type webPrefilter struct {
	db       minirehs.Database
	compiled []int
	force    []int
	fallback bool
}

var (
	webPrefilterCache  sync.Map
	webPrefilterFlight singleflight.Group
	webPrefilterBuilds atomic.Int32
)

type resourceGetterSlot struct {
	once sync.Once
	res  *rule.MatchResource
	err  error
}

func cachingGetter(getter func(string) (*rule.MatchResource, error)) func(string) (*rule.MatchResource, error) {
	var slots sync.Map
	return func(path string) (*rule.MatchResource, error) {
		cached, _ := slots.LoadOrStore(path, &resourceGetterSlot{})
		slot := cached.(*resourceGetterSlot)
		// Cache failures too: hundreds of favicon rules must not each retry
		// the same slow or unavailable path on a single target.
		slot.once.Do(func() {
			slot.res, slot.err = getter(path)
		})
		return slot.res, slot.err
	}
}

func rootRegexp(r *rule.FingerPrintRule) (string, bool) {
	if r == nil || r.Method != "regexp" || r.MatchParam == nil {
		return "", false
	}
	if r.WebPath != "" && r.WebPath != "/" {
		return "", false
	}
	if r.MatchParam.RegexpPattern == "" {
		return "", false
	}
	return r.WebPath, true
}

func rootRegexpPaths(rules []*rule.FingerPrintRule) []string {
	seen := map[string]struct{}{}
	var paths []string
	for _, r := range rules {
		path, ok := rootRegexp(r)
		if !ok {
			continue
		}
		if _, dup := seen[path]; dup {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths
}

func filterRootWebRules(rules []*rule.FingerPrintRule, pages map[string][]byte, failed map[string]bool) []*rule.FingerPrintRule {
	keep := make([]bool, len(rules))
	grouped := map[string][]int{}
	for i, r := range rules {
		path, ok := rootRegexp(r)
		if !ok {
			keep[i] = true
			continue
		}
		grouped[path] = append(grouped[path], i)
	}
	if len(grouped) == 0 {
		return rules
	}
	for path, indexes := range grouped {
		if failed[path] {
			for _, idx := range indexes {
				keep[idx] = true
			}
			continue
		}
		group := make([]*rule.FingerPrintRule, len(indexes))
		for i, idx := range indexes {
			group[i] = rules[idx]
		}
		allowed := webPrefilterFor(group).allows(pages[path], group)
		allowedSet := make(map[*rule.FingerPrintRule]struct{}, len(allowed))
		for _, r := range allowed {
			allowedSet[r] = struct{}{}
		}
		for _, idx := range indexes {
			if _, ok := allowedSet[rules[idx]]; ok {
				keep[idx] = true
			}
		}
	}
	out := make([]*rule.FingerPrintRule, 0, len(rules))
	for i, r := range rules {
		if keep[i] {
			out = append(out, r)
		}
	}
	return out
}

func webRuleSetKey(rules []*rule.FingerPrintRule) string {
	var b strings.Builder
	for _, r := range rules {
		b.WriteString(fmt.Sprintf("%p,", r))
	}
	return b.String()
}

func webPrefilterFor(rules []*rule.FingerPrintRule) *webPrefilter {
	if len(rules) == 0 {
		return &webPrefilter{fallback: true}
	}
	key := webRuleSetKey(rules)
	if cached, ok := webPrefilterCache.Load(key); ok {
		return cached.(*webPrefilter)
	}
	v, _, _ := webPrefilterFlight.Do(key, func() (interface{}, error) {
		if cached, ok := webPrefilterCache.Load(key); ok {
			return cached.(*webPrefilter), nil
		}
		compiled := compileWebPrefilter(rules)
		webPrefilterCache.Store(key, compiled)
		return compiled, nil
	})
	filter, _ := v.(*webPrefilter)
	if filter == nil {
		return &webPrefilter{fallback: true}
	}
	return filter
}

func compileWebPrefilter(rules []*rule.FingerPrintRule) *webPrefilter {
	webPrefilterBuilds.Add(1)
	filter := &webPrefilter{}
	patterns := make([]minirehs.Pattern, 0, len(rules))
	for i, r := range rules {
		expr := ""
		if r != nil && r.MatchParam != nil {
			expr = r.MatchParam.RegexpPattern
		}
		if expr == "" || !regexp_utils.NewYakRegexpUtils(expr).CanUse() {
			filter.force = append(filter.force, i)
			continue
		}
		patterns = append(patterns, minirehs.Pattern{
			ID:   minirehs.PatternID(len(filter.compiled)),
			Expr: expr,
		})
		filter.compiled = append(filter.compiled, i)
	}
	if len(patterns) == 0 {
		return filter
	}
	// 同一批首页正则的自动机写入本机镜像。进程重启后加载，不再重新编译。
	db, err := imagecache.Open("web", patterns,
		minirehs.WithBackend(minirehs.BackendMVS),
		minirehs.WithReportLocation(false),
		minirehs.WithLogger(webPrefilterLog{}),
	)
	if err != nil {
		log.Warnf("web fingerprint prefilter fell back to per-rule match: %v", err)
		filter.fallback = true
		filter.compiled = nil
		return filter
	}
	filter.db = db
	if len(rules) >= 64 {
		log.Debugf("web fingerprint prefilter ready: rules=%d", len(rules))
	}
	return filter
}

func (p *webPrefilter) allows(page []byte, rules []*rule.FingerPrintRule) []*rule.FingerPrintRule {
	if p == nil || p.fallback || p.db == nil && len(p.force) != len(rules) {
		return rules
	}
	keep := make([]bool, len(rules))
	for _, idx := range p.force {
		if idx >= 0 && idx < len(keep) {
			keep[idx] = true
		}
	}
	if p.db != nil {
		scratch, err := p.db.NewScratch()
		if err != nil {
			return rules
		}
		defer scratch.Close()
		if err := p.db.Scan(page, scratch, func(m minirehs.Match) bool {
			id := int(m.ID)
			if id >= 0 && id < len(p.compiled) {
				idx := p.compiled[id]
				if idx >= 0 && idx < len(keep) {
					keep[idx] = true
				}
			}
			return true
		}); err != nil {
			return rules
		}
	}
	selected := make([]*rule.FingerPrintRule, 0, len(rules))
	for i, rule := range rules {
		if keep[i] {
			selected = append(selected, rule)
		}
	}
	return selected
}
