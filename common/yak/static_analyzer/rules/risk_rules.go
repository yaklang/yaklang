package rules

import (
	"strconv"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/plugin_type"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var RiskTypeFuncs = []string{"risk.type", "risk.typeVerbose"}

func init() {
	plugin_type.RegisterCheckRuler(plugin_type.PluginTypeYak, RuleRisk)
	plugin_type.RegisterCheckRuler(plugin_type.PluginTypeYak, RuleRiskTypeVerbose)
}

// 检查 risk 是否符合规范
func RuleRisk(prog *ssaapi.Program) *result.StaticAnalyzeResults {
	ret := result.NewStaticAnalyzeResults("risk check")

	checkRiskOption := func(funcName string) {
		prog.Ref(funcName).GetUsers().Filter(func(v *ssaapi.Value) bool {
			return v.IsCall() && v.IsReachable() != -1
		}).ForEach(func(v *ssaapi.Value) {
			RiskDescription := false
			RiskSolution := false
			RiskCVE := false
			ops := v.GetOperands()
			for i := 2; i < len(ops); i++ {
				// log.Infof("ops %v", ops[i])
				opt := ops[i]
				optFuncName := opt.GetOperand(0).GetName()
				log.Infof("optFuncName %v", optFuncName)

				if optFuncName == "risk.description" {
					RiskDescription = true
				}

				if optFuncName == "risk.solution" {
					RiskSolution = true
				}

				if optFuncName == "risk.cve" {
					RiskCVE = true
				}

			}
			if !(RiskCVE || (RiskDescription && RiskSolution)) {
				ret.NewError(funcName+" should be called with (risk.description and risk.solution) or risk.cve", v)
			}
		})
	}

	checkRiskOption("risk.NewRisk")
	checkRiskOption("risk.CreateRisk")

	// check CreateRisk is saved ?
	prog.Ref("risk.CreateRisk").GetUsers().Filter(func(v *ssaapi.Value) bool {
		return v.IsCall() && v.IsReachable() != -1
	}).ForEach(func(v *ssaapi.Value) {
		// this v is risk.CreateRisk()
		flag := false
		v.GetUsers().Filter(func(v *ssaapi.Value) bool {
			return v.IsCall()
		}).ForEach(func(v *ssaapi.Value) {
			// this is user of risk.CreateRisk()
			if v.GetCallee().GetName() == "risk.Save" {
				flag = true
			}
		})
		if !flag {
			ret.NewError(ErrorRiskCreateNotSave(), v)
		}
	})

	return ret
}

// 检查 risk.type 是否符合规范
func RuleRiskTypeVerbose(prog *ssaapi.Program) *result.StaticAnalyzeResults {
	ret := result.NewStaticAnalyzeResults("risk verbose check")

	for _, funcName := range RiskTypeFuncs {
		prog.Ref(funcName).GetUsers().Filter(func(v *ssaapi.Value) bool {
			return v.IsCall() && v.IsReachable() != -1
		}).ForEach(func(v *ssaapi.Value) {
			ops := v.GetOperands()
			if len(ops) != 2 {
				return
			}
			constValue := ops[1].String()
			unquoted, err := strconv.Unquote(constValue)
			if err == nil {
				constValue = unquoted
			}

			if !lo.Contains(yakit.RiskTypes, constValue) {
				ret.NewWarn(WarnInvalidRiskTypeVerbose(), v)
			}
		})
	}

	return ret
}

func WarnInvalidRiskTypeVerbose() string {
	return "risk type invalid, will be replaced with `其他`"
}

func ErrorRiskCreateNotSave() string {
	return "risk.CreateRisk should be saved use `risk.Save`"
}

func ErrorRiskCheck() string {
	return "risk.NewRisk should be called with (risk.description and risk.solution) or risk.cve"
}
