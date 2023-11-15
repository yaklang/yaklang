package yakgrpc

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type YaklangInformationResponse struct {
	prog         *ssaapi.Program
	Suggestion   []string
	CliParameter []*CliParameter `json:"cliParameter"`
	Risk         []*ypb.RiskInfo
}

func newYaklangInformationResponse() *YaklangInformationResponse {
	return &YaklangInformationResponse{
		Suggestion:   make([]string, 0),
		CliParameter: make([]*CliParameter, 0),
		Risk:         make([]*ypb.RiskInfo, 0),
	}
}

func (rsp *YaklangInformationResponse) ToGrpcModule() (*ypb.YaklangInspectInformationResponse, error) {
	bCP, err := json.Marshal(rsp.CliParameter)
	if err != nil {
		return nil, err
	}
	return &ypb.YaklangInspectInformationResponse{
		SuggestionMessage: rsp.Suggestion,
		CliParameters:     utils.UnsafeBytesToString(bCP),
		RiskInformations:  rsp.Risk,
	}, nil
}

func fromGrpcModuleToYaklangInformationResponse(rsp *ypb.YaklangInspectInformationResponse) (*YaklangInformationResponse, error) {
	r := newYaklangInformationResponse()
	r.Suggestion = rsp.SuggestionMessage
	r.Risk = rsp.RiskInformations
	err := json.Unmarshal(utils.UnsafeStringToBytes(rsp.CliParameters), &r.CliParameter)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (r *YaklangInformationResponse) addCliParameter(param *CliParameter) {
	r.CliParameter = append(r.CliParameter, param)
}

func (r *YaklangInformationResponse) addSuggestion(suggestion string) {
	r.Suggestion = append(r.Suggestion, suggestion)
}

func (r *YaklangInformationResponse) addRisk(risk *ypb.RiskInfo) {
	r.Risk = append(r.Risk, risk)
}

type CliParameter struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Help     string `json:"help"`
	Required bool   `json:"required"`
	Default  any    `json:"default"`
}

func newCliParameter(typ, name string) *CliParameter {
	return &CliParameter{
		Name:     name,
		Type:     typ,
		Help:     "",
		Required: false,
		Default:  nil,
	}
}

func newRiskInfo() *ypb.RiskInfo {
	return &ypb.RiskInfo{}
}

func (r *YaklangInformationResponse) ParseCliParameter() {
	prog := r.prog
	// prog.Show()

	getConstString := func(v *ssaapi.Value) string {
		if str, ok := v.GetConstValue().(string); ok {
			return str
		}
		return ""
	}

	handleOption := func(cli *CliParameter, opt *ssaapi.Value) {
		// opt.ShowUseDefChain()
		if !opt.IsCall() {
			// skip no function call
			return
		}

		// check option function, get information
		switch opt.GetOperand(0).String() {
		case "cli.setHelp":
			cli.Help = getConstString(opt.GetOperand(1))
		case "cli.setRequired":
			cli.Required = getConstString(opt.GetOperand(1)) == "true"
		case "cli.setDefault":
			cli.Default = opt.GetOperand(1).GetConstValue()
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
			// add
			r.addCliParameter(cli)
		})
	}

	parseCliFunction("cli.String", "string")
	parseCliFunction("cli.Bool", "bool")
	parseCliFunction("cli.Int", "int")
	parseCliFunction("cli.Integer", "int")
	parseCliFunction("cli.Double", "float")
	parseCliFunction("cli.Float", "float")
	parseCliFunction("cli.Url", "urls")
	parseCliFunction("cli.Urls", "urls")
	parseCliFunction("cli.Port", "port")
	parseCliFunction("cli.Ports", "port")
	parseCliFunction("cli.Net", "hosts")
	parseCliFunction("cli.Network", "hosts")
	parseCliFunction("cli.Host", "hosts")
	parseCliFunction("cli.Hosts", "hosts")
	parseCliFunction("cli.File", "file")
	parseCliFunction("cli.FileOrContent", "file_or_content")
	parseCliFunction("cli.LineDict", "file-or-content")
	parseCliFunction("cli.YakitPlugin", "yakit-plugin")
	parseCliFunction("cli.StringSlice", "string-slice")
}

func (r *YaklangInformationResponse) ParseRiskInfo() {
	prog := r.prog

	getConstString := func(v *ssaapi.Value) string {
		if v.IsConstInst() {
			if str, ok := v.GetConstValue().(string); ok {
				return str
			}
		}
		//TODO: handler value with other opcode
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

	handleOption := func(riskInfo *ypb.RiskInfo, call *ssaapi.Value) {
		if !call.IsCall() {
			return
		}
		switch call.GetOperand(0).String() {
		case "risk.severity", "risk.level":
			riskInfo.Level = handleRiskLevel(getConstString(call.GetOperand(1)))
		case "risk.cve":
			riskInfo.Cve = call.GetOperand(1).String()
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
			r.addRisk(riskInfo)
		})
	}

	parseRiskFunction("risk.CreateRisk", 1)
	parseRiskFunction("risk.NewRisk", 1)
}

func (s *Server) YaklangInspectInformation(ctx context.Context, req *ypb.YaklangInspectInformationRequest) (*ypb.YaklangInspectInformationResponse, error) {
	rsp := newYaklangInformationResponse()
	rsp.prog = yak.Parse(req.YakScriptCode)
	if rsp.prog == nil {
		return nil, errors.New("ssa parse error")
	}
	if req.StartPos != nil || req.EndPos != nil {
		// TODO: get suggestion
	} else {
		rsp.ParseCliParameter()
		rsp.ParseRiskInfo()
	}
	return rsp.ToGrpcModule()
}
