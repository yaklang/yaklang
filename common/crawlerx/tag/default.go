package tag

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"strings"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const rulePath string = "/Users/chenyangbao/Project/yak/common/crawlerx/tag/rules/rule.yml"

type TDetect struct {
	//resHeader *http.Header
	//resBody   string
	rulePath string
	rules    Rules
}

type Rule struct {
	Name   string      `yaml:"NAME"`
	Info   []*RuleInfo `yaml:"RULES"`
	Method string      `yaml:"METHOD"`
}

type RuleInfo struct {
	RuleType   string `yaml:"RULE_TYPE"`
	RuleStr    string `yaml:"RULE"`
	OriginData string `yaml:"ORIGIN"`
	Key        string `yaml:"KEY"`
}

type FlowData interface {
	GetData(string) interface{}
}

type Rules []*Rule

func (tDtect *TDetect) SetRulePath(rulePath string) {
	tDtect.rulePath = rulePath
}

func (tDetect *TDetect) GetTag(data FlowData) []string {
	tags := make([]string, 0)
	for _, rule := range tDetect.rules {
		if utils.StringArrayContains(tags, rule.Name) {
			continue
		}
		var status bool
		for _, info := range rule.Info {
			data := data.GetData(info.OriginData)
			//if info.OriginData == "response.requestData" && data != "" && data != nil {
			//	fmt.Println(data)
			//}
			if data == "" {
				status = false
				break
			}
			if d, ok := data.(map[string]string); ok && len(d) == 0 {
				status = false
				break
			}
			ruleType := strings.ToLower(info.RuleType)
			switch ruleType {
			case "re":
				status = reCheck(data, info)
			case "json":
				status = jsonCheck(data, info)
			case "script":
				status = scriptCheck(data, info)
			case "xpath":
				status = xpathCheck(data, info)
			default:
				status = false
			}
			if !status {
				break
			}
		}
		if status {
			tags = append(tags, rule.Name)
		}
	}
	return tags
}

func (tDetect *TDetect) Init() {
	rules := ReadRules(tDetect.rulePath)
	tDetect.rules = rules
}

func ReadRules(path string) Rules {
	var rPath string
	if path == "" {
		rPath = rulePath
	} else {
		rPath = path
	}
	yamlFile, err := ioutil.ReadFile(rPath)
	if err != nil {
		log.Errorf("read error: %s", err)
		return nil
	}
	var r Rules
	err = yaml.Unmarshal(yamlFile, &r)
	if err != nil {
		log.Errorf("unmarshal yaml error: %s", err)
		return nil
	}
	return r
}
