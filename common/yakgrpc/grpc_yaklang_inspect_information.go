package yakgrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	pta "github.com/yaklang/yaklang/common/yak/plugin_type_analyzer"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type PluginParamSelect struct {
	Double bool                    `json:"double"`
	Data   []PluginParamSelectData `json:"data"`
}

type PluginParamSelectData struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

func cliParam2grpc(params []*pta.CliParameter) []*ypb.YakScriptParam {
	ret := make([]*ypb.YakScriptParam, 0, len(params))

	for _, param := range params {
		defaultValue := ""
		if param.Default != nil {
			defaultValue = fmt.Sprintf("%v", param.Default)
		}
		extra := []byte{}
		if param.Type == "select" {
			paramSelect := &PluginParamSelect{
				Double: param.MultipleSelect,
				Data:   make([]PluginParamSelectData, 0),
			}
			for k, v := range param.SelectOption {
				paramSelect.Data = append(paramSelect.Data, PluginParamSelectData{
					Label: k,
					Value: v,
				})
			}
			extra, _ = json.Marshal(paramSelect)
		}

		ret = append(ret, &ypb.YakScriptParam{
			Field:        param.Name,
			DefaultValue: string(defaultValue),
			TypeVerbose:  param.Type,
			FieldVerbose: param.NameVerbose,
			Help:         param.Help,
			Required:     param.Required,
			Group:        param.Group,
			ExtraSetting: string(extra),
		})
	}

	return ret
}

func (s *Server) YaklangInspectInformation(ctx context.Context, req *ypb.YaklangInspectInformationRequest) (*ypb.YaklangInspectInformationResponse, error) {
	ret := &ypb.YaklangInspectInformationResponse{}
	prog := ssaapi.Parse(req.YakScriptCode, pta.GetPluginSSAOpt(req.YakScriptType)...)
	if prog.IsNil() {
		return nil, errors.New("ssa parse error")
	}
	ret.CliParameter = cliParam2grpc(pta.ParseCliParameter(prog))

	return ret, nil
}

// compare p1 and p2 all field
func CompareParameter(p1, p2 *ypb.YakScriptParam) bool {
	if p1.Field != p2.Field {
		return false
	}
	if p1.FieldVerbose != p2.FieldVerbose {
		return false
	}
	if p1.TypeVerbose != p2.TypeVerbose {
		return false
	}
	if p1.Help != p2.Help {
		return false
	}
	if p1.DefaultValue != p2.DefaultValue {
		return false
	}
	if p1.Group != p2.Group {
		return false
	}
	if p1.Required != p2.Required {
		return false
	}
	if p1.ExtraSetting != p2.ExtraSetting {
		return false
	}
	return true
}

func getCliCodeFromParam(params []*ypb.YakScriptParam) string {
	code := ""
	for _, para := range params {
		// switch para.
		Option := make([]string, 0)
		cliFunction := ""
		var cliDefault string
		switch para.TypeVerbose {
		case "string":
			cliFunction = "String"
			if para.DefaultValue != "" {
				cliDefault = fmt.Sprintf("cli.setDefault(%#v)", para.DefaultValue)
			}
		case "uint":
			cliFunction = "Int"
			if para.DefaultValue != "" {
				i, err := strconv.ParseInt(para.DefaultValue, 10, 64)
				if err == nil {
					cliDefault = fmt.Sprintf("cli.setDefault(%d)", i)
				}
			}
		case "boolean":
			cliFunction = "Bool"
			if para.DefaultValue != "" {
				b, err := strconv.ParseBool(para.DefaultValue)
				if err == nil {
					cliDefault = fmt.Sprintf("cli.setDefault(%t)", b)
				}
			}
		case "float":
			cliFunction = "Float"
			if para.DefaultValue != "" {
				f, err := strconv.ParseFloat(para.DefaultValue, 64)
				if err == nil {
					cliDefault = fmt.Sprintf("cli.setDefault(%f)", f)
				}
			}
		case "upload-path":
			cliFunction = "File"
		case "http-packet":
			cliFunction = "HTTPPacket"
			if para.DefaultValue != "" {
				cliDefault = fmt.Sprintf("cli.setDefault(%#v)", para.DefaultValue)
			}
		case "yak":
			cliFunction = "YakCode"
			if para.DefaultValue != "" {
				cliDefault = fmt.Sprintf("cli.setDefault(%#v)", para.DefaultValue)
			}
		case "text":
			cliFunction = "Text"
			if para.DefaultValue != "" {
				cliDefault = fmt.Sprintf("cli.setDefault(%#v)", para.DefaultValue)
			}
		case "urls":
			cliFunction = "Urls"
			if para.DefaultValue != "" {
				cliDefault = fmt.Sprintf("cli.setDefault(%#v)", para.DefaultValue)
			}
		case "ports":
			cliFunction = "Ports"
			if para.DefaultValue != "" {
				cliDefault = fmt.Sprintf("cli.setDefault(%#v)", para.DefaultValue)
			}
		case "hosts":
			cliFunction = "Hosts"
			if para.DefaultValue != "" {
				cliDefault = fmt.Sprintf("cli.setDefault(%#v)", para.DefaultValue)
			}

		case "select":
			cliFunction = "StringSlice"
			if para.ExtraSetting != "" {
				var dataSelect *PluginParamSelect
				json.Unmarshal([]byte(para.ExtraSetting), &dataSelect)
				Option = append(Option, fmt.Sprintf(`cli.setMultipleSelect(%t)`, dataSelect.Double))
				for _, v := range dataSelect.Data {
					Option = append(Option, fmt.Sprintf(`cli.setSelectOption(%#v, %#v)`, v.Label, v.Value))
				}
			}
		default:
			cliFunction = "Undefine-" + para.TypeVerbose
		}

		if cliDefault != "" {
			Option = append(Option, cliDefault)
		}

		if para.Help != "" {
			Option = append(Option, fmt.Sprintf(`cli.setHelp(%#v)`, para.Help))
		}
		if para.FieldVerbose != "" && para.FieldVerbose != para.Field {
			Option = append(Option, fmt.Sprintf(`cli.setVerboseName(%#v)`, para.FieldVerbose))
		}
		if para.Group != "" {
			Option = append(Option, fmt.Sprintf(`cli.setCliGroup(%#v)`, para.Group))
		}
		if para.Required {
			Option = append(Option, fmt.Sprintf(`cli.setRequired(%t)`, para.Required))
		}

		str := fmt.Sprintf(`cli.%s(%#v, %s)`, cliFunction, para.Field, strings.Join(Option, ","))
		code += str + "\n"
	}
	return code
}

func getParameterFromParamJson(j string) ([]*ypb.YakScriptParam, error) {
	params, err := strconv.Unquote(j)
	if err != nil {
		return nil, utils.Wrapf(err, "unquote error")
	}
	var paras []*ypb.YakScriptParam
	err = json.Unmarshal([]byte(params), &paras)
	if err != nil {
		return nil, utils.Wrapf(err, "unmarshal error")
	}
	// return GetCliCodeFromParam(paras), nil
	return paras, nil
}

func getNeedReturn(script *yakit.YakScript) ([]*ypb.YakScriptParam, error) {
	prog := ssaapi.Parse(script.Content, pta.GetPluginSSAOpt("yak")...)
	if prog.IsNil() {
		return nil, errors.New("ssa parse error")
	}
	codeParameter := cliParam2grpc(pta.ParseCliParameter(prog))
	databaseParameter, err := getParameterFromParamJson(script.Params)
	if err != nil {
		return nil, utils.Wrapf(err, "get cli code from param json error")
	}
	needReturn := make([]*ypb.YakScriptParam, 0)
	// need := false
	// sort codeParameter and databaseParameter by field
	sort.Slice(codeParameter, func(i, j int) bool {
		return codeParameter[i].Field < codeParameter[j].Field
	})
	sort.Slice(databaseParameter, func(i, j int) bool {
		return databaseParameter[i].Field < databaseParameter[j].Field
	})

	// compare codeParameter and databaseParameter, and add rest in databaseParameter to needReturn
	for i := 0; i < len(codeParameter) && i < len(databaseParameter); i++ {
		if !CompareParameter(codeParameter[i], databaseParameter[i]) {
			needReturn = append(needReturn, databaseParameter[i])
			// need = true
		}
	}
	for i := len(codeParameter); i < len(databaseParameter); i++ {
		needReturn = append(needReturn, databaseParameter[i])
	}
	return needReturn, nil

}

func (s *Server) YaklangGetCliCodeFromDatabase(ctx context.Context, req *ypb.YaklangGetCliCodeFromDatabaseRequest) (*ypb.YaklangGetCliCodeFromDatabaseResponse, error) {
	name := req.GetScriptName()
	if name == "" {
		return nil, utils.Errorf("script name is empty")
	}
	script, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), name)
	if err != nil {
		return nil, utils.Wrapf(err, "get script %s error", name)
	}
	if script.Type != "yak" {
		return nil, utils.Errorf("script %s is not yak script", name)
	}
	var code string
	if need, err := getNeedReturn(script); err == nil {
		code = fmt.Sprintf(`/*
// this code generated by yaklang from database
%s
*/`, getCliCodeFromParam(need))
	} else {
		log.Error(err)
		code = ""
	}
	return &ypb.YaklangGetCliCodeFromDatabaseResponse{
		Code:       code,
		NeedHandle: len(code) != 0,
	}, nil
}
