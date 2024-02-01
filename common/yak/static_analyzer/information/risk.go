package information

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type RiskInfo struct {
	Level             string
	CVE               string
	Type, TypeVerbose string
	Description       string
	Solution          string
}

func newRiskInfo() *RiskInfo {
	return &RiskInfo{
		Level:       "",
		CVE:         "",
		Type:        "", // ignore
		TypeVerbose: "",
		Description: "",
		Solution:    "",
	}
}

func ParseRiskInfo(prog *ssaapi.Program) []*RiskInfo {
	ret := make([]*RiskInfo, 0)
	getConstString := func(v *ssaapi.Value) string {
		if v.IsConstInst() {
			if str, ok := v.GetConstValue().(string); ok {
				return str
			}
		}
		// TODO: handler value with other opcode
		return ""
	}

	handleRiskLevel := func(level string) string {
		switch level {
		case "high":
			return "high"
		case "critical", "panic", "fatal":
			return "critical"
		case "warning", "warn", "middle", "medium":
			return "warning"
		case "info", "debug", "trace", "fingerprint", "note", "fp":
			return "info"
		case "low", "default":
			return "low"
		default:
			return "low"
		}
	}

	handleOption := func(riskInfo *RiskInfo, call *ssaapi.Value) {
		if !call.IsCall() {
			return
		}
		arg1 := getConstString(call.GetOperand(1))
		switch call.GetOperand(0).String() {
		case "risk.severity", "risk.level":
			riskInfo.Level = handleRiskLevel(arg1)
		case "risk.cve":
			riskInfo.CVE = arg1
		// TODO: handler this type and typeVerbose
		case "risk.type":
			riskInfo.Type = arg1
			riskInfo.TypeVerbose = yakit.RiskTypeToVerbose(riskInfo.Type)
		case "risk.typeVerbose":
			riskInfo.TypeVerbose = yakit.RiskTypeToVerbose(riskInfo.Type)
		case "risk.description":
			riskInfo.Description = arg1
		case "risk.solution":
			riskInfo.Solution = arg1
		}
	}

	parseRiskFunction := func(name string, OptIndex int) {
		prog.Ref(name).GetUsers().Filter(func(v *ssaapi.Value) bool {
			return v.IsCall() && v.IsReachable() != -1
		}).ForEach(func(v *ssaapi.Value) {
			riskInfo := newRiskInfo()
			optLen := len(v.GetOperands())
			for i := OptIndex; i < optLen; i++ {
				handleOption(riskInfo, v.GetOperand(i))
			}
			ret = append(ret, riskInfo)
		})
	}

	parseRiskFunction("risk.CreateRisk", 1)
	parseRiskFunction("risk.NewRisk", 1)

	return ret
}
