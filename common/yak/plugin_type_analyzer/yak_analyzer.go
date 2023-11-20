package plugin_type_analyzer

import (
	"reflect"

	"github.com/yaklang/yaklang/common/yak/plugin_type_analyzer/rules"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

type yakAnalyzer struct{}

var _ PluginTypeAnalyzer = (*yakAnalyzer)(nil)

func (y *yakAnalyzer) GetTypeSSAOpt() []ssaapi.Option {
	opts := make([]ssaapi.Option, 0)
	// yak function table
	symbol := yaklang.New().GetFntable()
	valueTable := make(map[string]interface{})
	// libTable := make(map[string]interface{})
	tmp := reflect.TypeOf(make(map[string]interface{}))
	for name, item := range symbol {
		itype := reflect.TypeOf(item)
		if itype == tmp {
			opts = append(opts, ssaapi.WithExternLib(name, item.(map[string]interface{})))
		} else {
			valueTable[name] = item
		}
	}

	// yak-main
	valueTable["YAK_DIR"] = ""
	valueTable["YAK_FILENAME"] = ""
	valueTable["YAK_MAIN"] = false
	valueTable["id"] = ""
	// param
	getParam := func(key string) interface{} {
		return nil
	}
	valueTable["getParam"] = getParam
	valueTable["getParams"] = getParam
	valueTable["param"] = getParam

	opts = append(opts, ssaapi.WithExternValue(valueTable))
	opts = append(opts, ssaapi.WithExternMethod(&rules.Builder{}))
	return opts
}
func (y *yakAnalyzer) CheckRule(prog *ssaapi.Program) {
	rules.RuleCliDefault(prog)
	rules.RuleCliParamName(prog)
	rules.RuleCliCheck(prog)
}

func (y *yakAnalyzer) GetTypeInfo(prog *ssaapi.Program) []*YaklangInfo {
	ret := make([]*YaklangInfo, 0)

	cliList := rules.ParseCliParameter(prog)
	cliInfo := NewYakLangInfo("cli")
	for _, cli := range cliList {
		cliInfo.AddKV(CliParameterToInformation(cli))
	}
	ret = append(ret, cliInfo)

	riskInfos := rules.ParseRiskInfo(prog)
	riskInfo := NewYakLangInfo("risk")
	for _, risk := range riskInfos {
		riskInfo.AddKV(RiskInfoToInformation(risk))
	}
	ret = append(ret, riskInfo)

	return ret
}

func CliParameterToInformation(c *rules.CliParameter) *YaklangInfoKV {
	ret := NewYaklangInfoKV("Name", c.Name)
	ret.AddExtern("Type", c.Type)
	ret.AddExtern("Help", c.Help)
	ret.AddExtern("Required", c.Required)
	ret.AddExtern("Default", c.Default)
	return ret
}

func RiskInfoToInformation(r *rules.RiskInfo) *YaklangInfoKV {
	ret := NewYaklangInfoKV("Name", "risk")
	ret.AddExtern("Level", r.Level)
	ret.AddExtern("CVE", r.CVE)
	ret.AddExtern("Type", r.Type)
	ret.AddExtern("TypeVerbose", r.TypeVerbose)
	return ret
}
