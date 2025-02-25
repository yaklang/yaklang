package yakgrpc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"github.com/yaklang/yaklang/embed"
)

func (s *Server) GetReverseShellProgramList(ctx context.Context, req *ypb.GetReverseShellProgramListRequest) (*ypb.GetReverseShellProgramListResponse, error) {
	reverseShellTemplatesData, err := embed.Asset("data/reverseShellTemplates.json.gz")
	if err != nil {
		return nil, err
	}
	templatesData := gjson.Parse(string(reverseShellTemplatesData))
	reverseShellCommands := templatesData.Get("reverseShellCommands")
	reverseType := req.GetCmdType()
	osName := req.GetSystem()
	shells := templatesData.Get("shells").Array()
	res := &ypb.GetReverseShellProgramListResponse{}
	for _, shell := range shells {
		res.ShellList = append(res.ShellList, shell.String())
	}
	for _, reverseShellCommandCfg := range reverseShellCommands.Array() {
		if reverseShellCommandCfg.Get("command").String() == "" {
			continue
		}
		metas := reverseShellCommandCfg.Get("meta").Array()
		lastMeta := ""
		premetas := []string{}
		if len(metas) > 0 {
			meta := metas[len(metas)-1]
			lastMeta = meta.String()
			for _, result := range metas[:len(metas)-1] {
				premetas = append(premetas, result.String())
			}
		}
		if (reverseType == "All" || lastMeta == reverseType) && (osName == "All" || utils.StringSliceContainsAll(premetas, strings.ToLower(osName))) {
			res.ProgramList = append(res.ProgramList, reverseShellCommandCfg.Get("name").String())
		}
	}
	return res, nil
}

func (s *Server) GenerateReverseShellCommand(ctx context.Context, req *ypb.GenerateReverseShellCommandRequest) (*ypb.GenerateReverseShellCommandResponse, error) {
	reverseShellTemplatesData, err := embed.Asset("data/reverseShellTemplates.json.gz")
	if err != nil {
		return nil, err
	}
	templatesData := gjson.Parse(string(reverseShellTemplatesData))
	//shells := templatesData.Get("shells")
	reverseShellCommands := templatesData.Get("reverseShellCommands")
	reverseType := req.GetCmdType()
	osName := req.GetSystem()
	prog := req.GetProgram()
	ip := req.GetIP()
	port := req.GetPort()
	shell := req.GetShellType()
	encode := req.GetEncode()
	result := ""
	for _, reverseShellCommandCfg := range reverseShellCommands.Array() {
		metas := reverseShellCommandCfg.Get("meta").Array()
		lastMeta := ""
		premetas := []string{}
		if len(metas) > 0 {
			meta := metas[len(metas)-1]
			lastMeta = meta.String()
			for _, result := range metas[:len(metas)-1] {
				premetas = append(premetas, result.String())
			}
		}
		if (reverseType == "All" || lastMeta == reverseType) && (osName == "All" || utils.StringSliceContainsAll(premetas, strings.ToLower(osName))) && reverseShellCommandCfg.Get("name").String() == prog {
			command := reverseShellCommandCfg.Get("command").String()
			command = strings.ReplaceAll(command, "{port}", fmt.Sprint(port))
			command = strings.ReplaceAll(command, "{ip}", ip)
			command = strings.ReplaceAll(command, "{shell}", shell)
			result = command
			break
		}
	}
	if result == "" {
		return nil, errors.New("generate command failed")
	}
	switch encode {
	case "Url":
		result = codec.EncodeUrlCode(result)
	case "DoubleUrl":
		result = codec.DoubleEncodeUrl(result)
	case "Base64":
		result = codec.EncodeBase64(result)
	case "None", "":

	default:
		return nil, fmt.Errorf("invalid encode type: %v", encode)
	}
	return &ypb.GenerateReverseShellCommandResponse{
		Status: &ypb.GeneralResponse{
			Ok: true,
		},
		Result: result,
	}, nil
}
