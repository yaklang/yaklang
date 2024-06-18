package fp

import (
	"github.com/yaklang/yaklang/common/fp/fingerprint/parsers"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/log"
	"io/ioutil"
	"os"
	"path/filepath"
)

func FileOrDirToWebRules(dir string) []*rule.FingerPrintRule {
	if dir == "" {
		return nil
	}

	log.Infof("loading user web-fingerprint path: %s", dir)

	pathInfo, err := os.Stat(dir)
	if err != nil {
		log.Errorf("open path[%s] failed: %s", dir, err)
		return nil
	}

	if !pathInfo.IsDir() {
		raw, err := ioutil.ReadFile(dir)
		if err != nil {
			log.Error(err)
			return nil
		}
		rules, err := parsers.ParseYamlRule(string(raw))
		if err != nil {
			log.Error(err)
		}
		return rules
	}

	var rules []*rule.FingerPrintRule
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() && err == nil {
			return nil
		}

		fileName := filepath.Join(dir, info.Name())
		log.Infof("loading: %s", fileName)
		raw, err := ioutil.ReadFile(fileName)
		if err != nil {
			log.Error(err)
			return nil
		}
		r, err := parsers.ParseYamlRule(string(raw))
		if err != nil {
			log.Error(err)
		}
		rules = append(rules, r...)
		return nil
	})
	if err != nil {
		log.Error(err)
	}
	return rules
}
