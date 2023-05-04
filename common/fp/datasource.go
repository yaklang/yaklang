package fp

import (
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/bindata"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"path"
	"sync"
)

var (
	DefaultNmapServiceProbeRules     map[*NmapProbe][]*NmapMatch
	DefaultNmapServiceProbeRulesOnce sync.Once
	DefaultWebFingerprintRules       []*webfingerprint.WebRule
	DefaultWebFingerprintRulesOnce   sync.Once
)

func loadDefaultNmapServiceProbeRules() (map[*NmapProbe][]*NmapMatch, error) {
	content, err := bindata.Asset("data/nfp.gz")
	if err != nil {
		return nil, errors.Errorf("get local service probe failed: %s", err)
	}

	content, err = utils.GzipDeCompress(content)
	if err != nil {
		return nil, utils.Errorf("decompress probe failed: %s", err)
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
	files, err := bindata.AssetDir(userDefinedPath)
	if err != nil {
		log.Infof("user defined rules is missed: %s", err)
		return rules, nil
	}

	for _, fileName := range files {
		absFileName := path.Join(userDefinedPath, fileName)
		content, err := bindata.Asset(absFileName)
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

func GetDefaultWebFingerprintRules() ([]*webfingerprint.WebRule, error) {
	var err error
	DefaultWebFingerprintRulesOnce.Do(func() {
		DefaultWebFingerprintRules, err = webfingerprint.LoadDefaultDataSource()
	})
	return DefaultWebFingerprintRules, err
}
