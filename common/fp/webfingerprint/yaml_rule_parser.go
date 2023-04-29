package webfingerprint

import (
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"yaklang/common/bindata"
	log "yaklang/common/log"
)

func GetYamlWebFingerprintRules(yamlFilePath string) ([]*WebRule, error) {
	fingerprintRules := []*WebRule{}

	getRulesOnDisk := func(path string) ([]byte, error) {
		ruleFileData, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, errors.Errorf("read fingerprint rules on disk[%s] fail: %s", path, err)
		}
		return ruleFileData, nil
	}

	getRulesInCode := func(path string) ([]byte, error) {
		ruleFileData, err := bindata.Asset(path)
		if err != nil {
			return nil, errors.Errorf("read fingerprint rules in code fail: %s", err)
		}
		return ruleFileData, nil
	}

	rulesOnDisk, err := getRulesOnDisk(yamlFilePath)
	if err != nil {
		log.Warnf("get rules on disk fail: %s", err)

		rulesInCode, err := getRulesInCode(yamlFilePath)
		if err != nil {
			return nil, errors.Errorf("get rules in code fail: %s", err)
			//log.Infof("use basic rules only.")
		} else {
			err = yaml.Unmarshal(rulesInCode, &fingerprintRules)
			if err != nil {
				return nil, errors.Errorf("unmarshal fingerprint rules in code error: %s", err)
			}
		}
	} else {
		err = yaml.Unmarshal(rulesOnDisk, &fingerprintRules)
		if err != nil {
			return nil, errors.Errorf("unmarshal fingerprint rules on disk error: %s", err)
		}
	}

	if len(fingerprintRules) > 0 {
		return fingerprintRules, nil
	}

	return nil, errors.New("no available rules")
}

func ParseWebFingerprintRules(raw []byte) ([]*WebRule, error) {
	var err error

	var rules []*WebRule
	err = yaml.Unmarshal(raw, &rules)
	if err != nil {
		var rule WebRule
		err = yaml.Unmarshal(raw, &rule)
		if err == nil {
			rules = append(rules, &rule)
		}
	}

	if len(rules) > 0 {
		return rules, nil
	}

	return nil, errors.Errorf("failed to parse to wfp.WebRule: \n%v\n", string(raw))
}
