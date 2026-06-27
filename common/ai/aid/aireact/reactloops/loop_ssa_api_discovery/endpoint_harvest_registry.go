package loop_ssa_api_discovery

import (
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type staticHarvesterFunc func(codeRoot string) ([]HarvestedEndpoint, error)

type registeredHarvester struct {
	sourceKey   string
	frameworkTags []string
	fn          staticHarvesterFunc
}

var (
	harvestRegMu  sync.RWMutex
	harvestByLang = map[ssaconfig.Language][]registeredHarvester{}
)

func registerStaticHarvester(lang ssaconfig.Language, sourceKey string, frameworkTags []string, fn staticHarvesterFunc) {
	if fn == nil || lang == "" || sourceKey == "" {
		return
	}
	harvestRegMu.Lock()
	defer harvestRegMu.Unlock()
	harvestByLang[lang] = append(harvestByLang[lang], registeredHarvester{sourceKey: sourceKey, frameworkTags: frameworkTags, fn: fn})
}

func staticHarvestersFor(lang ssaconfig.Language) []registeredHarvester {
	harvestRegMu.RLock()
	defer harvestRegMu.RUnlock()
	out := make([]registeredHarvester, len(harvestByLang[lang]))
	copy(out, harvestByLang[lang])
	return out
}

// staticHarvestersForFrameworks filters harvesters by detected framework IDs from project_profile.
func staticHarvestersForFrameworks(lang ssaconfig.Language, frameworks []string) []registeredHarvester {
	all := staticHarvestersFor(lang)
	if len(frameworks) == 0 {
		return all
	}
	fwSet := map[string]struct{}{}
	for _, f := range frameworks {
		fwSet[strings.ToLower(strings.TrimSpace(f))] = struct{}{}
	}
	var out []registeredHarvester
	for _, h := range all {
		if len(h.frameworkTags) == 0 {
			continue
		}
		for _, tag := range h.frameworkTags {
			if _, ok := fwSet[strings.ToLower(tag)]; ok {
				out = append(out, h)
				break
			}
		}
	}
	if len(out) == 0 {
		return all
	}
	return out
}

// LanguageHasStaticHarvester 若会话 language 在 SSA 枚举下且已注册至少一个静态抽取器，返回 true。
func LanguageHasStaticHarvester(sessLanguage string) bool {
	l, err := ssaconfig.ValidateLanguage(sessLanguage)
	if err != nil || l == "" {
		return false
	}
	harvestRegMu.RLock()
	defer harvestRegMu.RUnlock()
	return len(harvestByLang[l]) > 0
}

func init() {
	registerStaticHarvester(ssaconfig.JAVA, "static_java_spring_annotations", []string{"spring"}, HarvestJavaSpringMappings)
	registerStaticHarvester(ssaconfig.GO, "static_go_http", nil, HarvestGoHTTPMappings)
	registerStaticHarvester(ssaconfig.JS, "static_javascript_http", nil, HarvestJavaScriptHTTPMappings)
	registerStaticHarvester(ssaconfig.TS, "static_typescript_http", nil, HarvestTypeScriptHTTPMappings)
	registerStaticHarvester(ssaconfig.PYTHON, "static_python_http", nil, HarvestPythonHTTPMappings)
	registerStaticHarvester(ssaconfig.PHP, "static_php_http", nil, HarvestPHPHTTPMappings)
}
