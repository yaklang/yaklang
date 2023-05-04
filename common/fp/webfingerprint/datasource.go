package webfingerprint

import (
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/bindata"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"path"
)

func LoadDefaultDataSource() ([]*WebRule, error) {
	content, err := bindata.Asset("data/fingerprint-rules.yml.gz")
	if err != nil {
		return nil, errors.Errorf("get local web fingerprint rules failed: %s", err)
	}

	content, err = utils.GzipDeCompress(content)
	if err != nil {
		return nil, utils.Errorf("web fp rules decompress failed: %s", err)
	}

	rules, err := ParseWebFingerprintRules(content)
	if err != nil {
		return nil, errors.Errorf("parse wappalyzer rules failed: %s", err)
	}
	rules = append(rules, DefaultWebFingerprintRules...)

	// 加载用户自定义的规则库
	userDefinedPath := "data/user-wfp-rules"
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

		subRules, err := ParseWebFingerprintRules(content)
		if err != nil {
			log.Warnf("parse FILE:%s failed: %s", absFileName, err)
			continue
		}

		rules = append(rules, subRules...)
	}

	return rules, nil
}
