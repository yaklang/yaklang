package coreplugin

import (
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strconv"
)

func ParseProjectWithAutoDetective(ctx context.Context, path, language string, compileImmediately bool, inputs ...map[string]any) (*autoDetectInfo, *ssaapi.Program, error) {
	pluginName := "SSA 项目探测"
	param := make(map[string]string)
	param["target"] = path
	param["language"] = language
	if compileImmediately {
		param["compile-immediately"] = strconv.FormatBool(compileImmediately)
	}
	for _, input := range inputs {
		for key, value := range input {
			param[key] = codec.AnyToString(value)
		}
	}

	var info *autoDetectInfo
	var err error

	err = yakgrpc.ExecScriptWithParam(ctx, pluginName, param,
		"", func(exec *ypb.ExecResult) error {
			if !exec.IsMessage {
				return nil
			}
			rawMsg := exec.GetMessage()
			var msg msg
			json.Unmarshal(rawMsg, &msg)
			// log.Infof("msg: %v", msg)
			if msg.Type == "log" && msg.Content.Level == "code" {
				// start compile
				err = json.Unmarshal([]byte(msg.Content.Data), &info)
				if err != nil {
					return err
				}
			}
			return nil
		},
	)
	if err != nil {
		return info, nil, err
	}
	if info == nil {
		return nil, nil, utils.Errorf("auto detective info is nil")
	}
	config := info.Config
	switch info.Error.Kind {
	case "languageNeedSelectException":
		return info, nil, utils.Errorf("language need select")
	case "fileNotFoundException":
		return info, nil, utils.Errorf("file not found")
	case "fileTypeException":
		return info, nil, utils.Errorf("input file type")
	case "connectFailException":
		return info, nil, utils.Errorf("connect fail")
	}

	if !compileImmediately {
		return info, nil, nil
	}

	programName := config.GetProgramName()
	if programName == "" {
		return info, nil, utils.Errorf("program name is empty")
	}

	if prog, err := ssaapi.FromDatabase(programName); err != nil {
		return info, nil, err
	} else {
		return info, prog, nil
	}
}

type msg struct {
	Type    string `json:"type"`
	Content struct {
		Level   string  `json:"level"`
		Data    string  `json:"data"`
		ID      string  `json:"id"`
		Process float64 `json:"progress"`
	}
}

type autoDetectInfo struct {
	*ssaconfig.Config
	FileCount          int  `json:"file_Count"`
	CompileImmediately bool `json:"compile_immediately"`
	Error              struct {
		Kind string `json:"kind"`
		Msg  string `json:"msg"`
	} `json:"error"`
}
