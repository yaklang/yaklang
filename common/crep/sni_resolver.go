package crep

import (
	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"
	"strings"
)

type sniGlobRule struct {
	pattern glob.Glob
	sni     string
}

type SNIResolver struct {
	overwriteSNI bool
	forceSNI     string
	exactMapping map[string]string
	globRules    []sniGlobRule
}

func NewSNIResolver(sniMapping map[string]string, overwriteSNI bool, forceSNI string) *SNIResolver {
	resolver := &SNIResolver{
		overwriteSNI: overwriteSNI,
		forceSNI:     forceSNI,
		exactMapping: make(map[string]string),
	}

	for host, sni := range sniMapping {
		if isGlobPattern(host) {
			pattern, err := glob.Compile(host, '.')
			if err != nil {
				log.Warnf("invalid glob pattern for SNI mapping: %s", host)
				resolver.exactMapping[host] = sni
				continue
			}
			resolver.globRules = append(resolver.globRules, sniGlobRule{
				pattern: pattern,
				sni:     sni,
			})
		} else {
			resolver.exactMapping[host] = sni
		}
	}

	return resolver
}

func (r *SNIResolver) Resolve(host string) *string {
	if r == nil {
		return nil
	}

	if sni, ok := r.exactMapping[host]; ok {
		return &sni
	}

	for _, rule := range r.globRules {
		if rule.pattern.Match(host) {
			return &rule.sni
		}
	}

	if r.overwriteSNI {
		return &r.forceSNI
	}

	return nil
}

func isGlobPattern(s string) bool {
	return strings.ContainsAny(s, "*?[")
}
