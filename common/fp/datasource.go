package fp

import (
	"github.com/yaklang/yaklang/common/fp/fingerprint/parsers"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
	"path"
	"sync"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/embed"
)

var (
	DefaultNmapServiceProbeRules     map[*NmapProbe][]*NmapMatch
	DefaultNmapServiceProbeRulesOnce sync.Once
	DefaultWebFingerprintRules       []*rule.FingerPrintRule
	OldDefaultWebFingerprintRules       []*webfingerprint.WebRule

	DefaultWebFingerprintRulesOnce   sync.Once
	OldDefaultWebFingerprintRulesOnce   sync.Once

)

func loadDefaultNmapServiceProbeRules() (map[*NmapProbe][]*NmapMatch, error) {
	content, err := embed.Asset("data/nfp.gz")
	if err != nil {
		return nil, errors.Errorf("get local service probe failed: %s", err)
	}

	rules, err := ParseNmapServiceProbeToRuleMap(content)
	if err != nil {
		return nil, errors.Errorf("parse nmap service probe failed: %s", err)
	}

	// 构建一个索引，从 string 到 NmapProbe
	var probes = map[string]*NmapProbe{}
	for probe, _ := range rules {
		probes[probe.Name+probe.Payload] = probe
	}
	strToNmapProbe := func(name string, probe *NmapProbe) *NmapProbe {
		ret, ok := probes[name]
		if !ok {
			return probe
		}
		return ret
	}

	// 加载用户自定义的规则库
	userDefinedPath := "data/user-fp-rules"
	files, err := embed.AssetDir(userDefinedPath)
	if err != nil {
		log.Infof("user defined rules is missed: %s", err)
		return rules, nil
	}

	for _, fileName := range files {
		absFileName := path.Join(userDefinedPath, fileName)
		content, err := embed.Asset(absFileName)
		if err != nil {
			log.Warnf("bindata fetch asset: %s failed: %s", absFileName, err)
			continue
		}

		subRules, err := ParseNmapServiceProbeToRuleMap(content)
		if err != nil {
			log.Warnf("parse FILE:%s failed: %s", absFileName, err)
			continue
		}

		for probe, matches := range subRules {
			//同名 且 同payload: "q|GET / HTTP/1.0\r\n\r\n|"的规则会进行合并，否则新增规则
			newProbe := strToNmapProbe(probe.Name+probe.Payload, probe)
			if originMatches, ok := rules[newProbe]; !ok {
				log.Debugf("user defined a new probe: %s, payload: %#v", newProbe.Name, newProbe.Payload)
				rules[newProbe] = matches
			} else {
				rules[newProbe] = append(originMatches, matches...)
			}
		}
	}

	return rules, nil
}

func GetDefaultNmapServiceProbeRules() (map[*NmapProbe][]*NmapMatch, error) {
	var err error
	DefaultNmapServiceProbeRulesOnce.Do(func() {
		DefaultNmapServiceProbeRules, err = loadDefaultNmapServiceProbeRules()
	})
	return DefaultNmapServiceProbeRules, err
}

func GetDefaultWebFingerprintRules() ([]*rule.FingerPrintRule, error) {
	var loadErr error
	DefaultWebFingerprintRulesOnce.Do(func() {
		content, err := embed.Asset("data/fingerprint-rules.yml.gz")
		if err != nil {
			loadErr = err
			return
		}
		buildinYamlRules, err := parsers.ParseYamlRule(string(content))
		if err != nil {
			loadErr = err
			return
		}
		buildinRules, err := parsers.ConvertOldYamlWebRuleToGeneralRule(webfingerprint.DefaultWebFingerprintRules)
		if err != nil {
			loadErr = err
			return
		}
		DefaultWebFingerprintRules = append(buildinRules, buildinYamlRules...)
	})
	return DefaultWebFingerprintRules, loadErr
}

func GetOldDefaultWebFingerprintRules() ([]*webfingerprint.WebRule, error) {
	var err error
	OldDefaultWebFingerprintRulesOnce.Do(func() {
		OldDefaultWebFingerprintRules, err = webfingerprint.LoadDefaultDataSource()
	})
	return OldDefaultWebFingerprintRules, err
}