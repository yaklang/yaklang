package rules

import (
	"github.com/yaklang/yaklang/common/yak/plugin_type_analyzer"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func init() {
	plugin_type_analyzer.RegisterCheckRuler("yak", RuleRisk)
}

// 检查 cli.risk 是否符合规范
func RuleRisk(prog *ssaapi.Program) {
	tag := "cli.risk"

	checkRisk := func(funcName string) {
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
				optFuncName := opt.GetOperand(0).String()
				// log.Infof("optFuncName %v", optFuncName)

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
				v.NewError(tag, funcName + " should be called with (risk.description and risk.solution) or risk.cve")
			}
		})
	}

	checkRisk("risk.NewRisk")
}

func ErrorRiskCheck() string {
	return "risk.NewRisk should be called with (risk.description and risk.solution) or risk.cve"
}
