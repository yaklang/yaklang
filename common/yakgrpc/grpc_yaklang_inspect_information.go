package yakgrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	pta "github.com/yaklang/yaklang/common/yak/plugin_type_analyzer"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
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
