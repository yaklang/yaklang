package tag

import (
	"golang.org/x/net/html"
	"regexp"
	"strings"
	"github.com/yaklang/yaklang/common/javascript/otto"
	"github.com/yaklang/yaklang/common/log"
)

func reCheck(data interface{}, ruleInfo *RuleInfo) bool {
	dataStr, ok := data.(string)
	if !ok {
		return false
	}
	r, err := regexp.Compile(ruleInfo.RuleStr)
	if err != nil {
		log.Errorf("reg exp %s error: %s", ruleInfo.RuleStr, err)
		return false
	}
	return r.MatchString(dataStr)
}

func jsonCheck(data interface{}, ruleInfo *RuleInfo) bool {
	dataMap, ok := data.(map[string]string)
	if !ok {
		return false
	}
	result, ok := dataMap[ruleInfo.Key]
	if !ok {
		return false
	}
	return result == ruleInfo.RuleStr
}

func scriptCheck(data interface{}, ruleInfo *RuleInfo) bool {
	vm := otto.New()
	vm.Set("ORIGIN", data)
	if strings.Contains(ruleInfo.RuleStr, "startsWith") {
		vm.Run(startsWithJS)
	}
	if strings.Contains(ruleInfo.RuleStr, "endsWith") {
		vm.Run(endsWithJS)
	}
	result, err := vm.Run(ruleInfo.RuleStr)
	if err != nil {
		log.Errorf("js code run error: %s", ruleInfo.RuleStr, err)
		return false
	}
	if !result.IsBoolean() {
		log.Errorf("result not boolean: %s", result)
		return false
	}
	boolResult, _ := result.ToBoolean()
	return boolResult
}

func xpathCheck(data interface{}, ruleInfo *RuleInfo) bool {
	dataStr, ok := data.(string)
	if !ok {
		return false
	}
	dataFlow := strings.NewReader(dataStr)
	tree, err := html.Parse(dataFlow)
	if err != nil {
		log.Errorf("html parse error: %s", err)
		return false
	}

	r, _ := regexp.Compile("([a-zA-Z]+)\\[([a-zA-Z]+)=([a-zA-Z]+)\\]")
	result := r.FindAllStringSubmatch(ruleInfo.RuleStr, -1)
	if len(result) == 0 {
		return false
	}
	d := result[0][1]
	k := result[0][2]
	v := result[0][3]
	status := visit(tree, d, k, v)
	return status
}
