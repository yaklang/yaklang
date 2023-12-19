package plugin_type_analyzer

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type CliParameter struct {
	Name           string
	NameVerbose    string
	Required       bool
	Type           string
	Default        any
	Help           string
	Group          string
	MultipleSelect bool
	SelectOption   map[string]string
}

func newCliParameter(typ, name string) *CliParameter {
	return &CliParameter{
		Name:           name,
		NameVerbose:    name,
		Required:       false,
		Type:           typ,
		Default:        nil,
		Help:           "",
		Group:          "",
		MultipleSelect: false,
		SelectOption:   nil,
	}
}

func ParseCliParameter(prog *ssaapi.Program) []*CliParameter {
	// prog.Show()
	ret := make([]*CliParameter, 0)

	getConstString := func(v *ssaapi.Value) string {
		if str, ok := v.GetConstValue().(string); ok {
			return str
		}
		return ""
	}
	getConstBool := func(v *ssaapi.Value) bool {
		if b, ok := v.GetConstValue().(bool); ok {
			return b
		}
		return false
	}

	handleOption := func(cli *CliParameter, opt *ssaapi.Value) {
		// opt.ShowUseDefChain()
		if !opt.IsCall() {
			// skip no function call
			return
		}
		arg1 := getConstString(opt.GetOperand(1))
		arg2 := getConstString(opt.GetOperand(2))

		// check option function, get information
		switch opt.GetOperand(0).String() {
		case "cli.setHelp":
			cli.Help = arg1
		case "cli.setRequired":
			cli.Required = getConstBool(opt.GetOperand(1))
		case "cli.setDefault":
			cli.Default = opt.GetOperand(1).GetConstValue()
		case "cli.setCliGroup":
			cli.Group = arg1
		case "cli.setVerboseName":
			cli.NameVerbose = arg1
		case "cli.setMultipleSelect": // only for `cli.StringSlice`
			if cli.Type != "select" {
				break
			}
			cli.MultipleSelect = getConstBool(opt.GetOperand(1))
		case "cli.setSelectOption": // only for `cli.StringSlice`
			if cli.Type != "select" {
				break
			}
			if cli.SelectOption == nil {
				cli.SelectOption = make(map[string]string)
			}
			cli.SelectOption[arg1] = arg2
		}
	}

	parseCliFunction := func(funName, typName string) {
		prog.Ref(funName).GetUsers().Filter(
			func(v *ssaapi.Value) bool {
				// only function call and must be reachable
				return v.IsCall() && v.IsReachable() != -1
			},
		).ForEach(func(v *ssaapi.Value) {
			// cli.String("arg1", opt...)
			// op(0) => cli.String
			// op(1) => "arg1"
			// op(2...) => opt
			name := v.GetOperand(1).String()
			if v.GetOperand(1).IsConstInst() {
				name = v.GetOperand(1).GetConstValue().(string)
			}
			cli := newCliParameter(typName, name)
			opLen := len(v.GetOperands())
			// handler option
			for i := 2; i < opLen; i++ {
				handleOption(cli, v.GetOperand(i))
			}
			ret = append(ret, cli)
		})
	}

	parseCliFunction("cli.String", "string")
	parseCliFunction("cli.Bool", "boolean") // "bool"
	parseCliFunction("cli.Int", "uint")     // "int"
	parseCliFunction("cli.Integer", "uint") // "int"
	parseCliFunction("cli.Double", "float")
	parseCliFunction("cli.Float", "float")

	parseCliFunction("cli.File", "upload-path")   // "file"
	parseCliFunction("cli.StringSlice", "select") // "string-slice"
	parseCliFunction("cli.YakCode", "yak")
	parseCliFunction("cli.HTTPPacket", "http-packet")
	parseCliFunction("cli.Text", "text")

	// TODO: un-support  in front-end
	parseCliFunction("cli.Url", "urls")
	parseCliFunction("cli.Urls", "urls")
	parseCliFunction("cli.Port", "port")
	parseCliFunction("cli.Ports", "port")
	parseCliFunction("cli.Net", "hosts")
	parseCliFunction("cli.Network", "hosts")
	parseCliFunction("cli.Host", "hosts")
	parseCliFunction("cli.Hosts", "hosts")
	parseCliFunction("cli.FileOrContent", "file_or_content")
	parseCliFunction("cli.LineDict", "file-or-content")

	return ret
}

type RiskInfo struct {
	Level             string
	CVE               string
	Type, TypeVerbose string
}

func newRiskInfo() *RiskInfo {
	return &RiskInfo{
		Level:       "",
		CVE:         "",
		Type:        "",
		TypeVerbose: "",
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
		switch call.GetOperand(0).String() {
		case "risk.severity", "risk.level":
			riskInfo.Level = handleRiskLevel(getConstString(call.GetOperand(1)))
		case "risk.cve":
			riskInfo.CVE = call.GetOperand(1).String()
		case "risk.type":
			riskInfo.Type = getConstString(call.GetOperand(1))
			riskInfo.TypeVerbose = yakit.RiskTypeToVerbose(riskInfo.Type)
		case "risk.typeVerbose":
			riskInfo.TypeVerbose = getConstString(call.GetOperand(1))
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
