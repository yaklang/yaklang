package coreplugin

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func ParseProjectWithAutoDetective(ctx context.Context, path, language string, compileImmediately bool, inputs ...map[string]any) (*programInfo, *ssaapi.Program, error) {
	pluginName := "SSA 项目探测"
	param := make(map[string]string)
	param["target"] = path
	param["language"] = language
	param["compile-immediately"] = strconv.FormatBool(compileImmediately)
	for _, input := range inputs {
		for key, value := range input {
			param[key] = codec.AnyToString(value)
		}
	}

	progInfo := &programInfo{}
	err := yakgrpc.ExecScriptWithParam(ctx, pluginName, param,
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
				json.Unmarshal([]byte(msg.Content.Data), progInfo)
				log.Infof("progInfo: %v", progInfo)
			}
			return nil
		},
	)

	if err != nil {
		return progInfo, nil, err
	}

	switch progInfo.Error.Kind {
	case "languageNeedSelectException":
		return progInfo, nil, utils.Errorf("language need select")
	case "fileNotFoundException":
		return progInfo, nil, utils.Errorf("file not found")
	case "fileTypeException":
		return progInfo, nil, utils.Errorf("input file type")
	case "connectFailException":
		return progInfo, nil, utils.Errorf("connect fail")
	}

	if !compileImmediately {
		return progInfo, nil, nil
	}

	if prog, err := ssaapi.FromDatabase(progInfo.ProgramName); err != nil {
		return progInfo, nil, err
	} else {
		return progInfo, prog, nil
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
type programInfo struct {
	ProgramName string `json:"program_name"`
	ProjectName string `json:"project_name"`
	Language    string `json:"language"`
	Info        struct {
		Kind      string `json:"kind"`
		LocalFile string `json:"local_file"`
		URL       string `json:"url"`
		Branch    string `json:"branch"`
		Path      string `json:"path"`
		Proxy     struct {
			Proxy bool   `json:"proxy"`
			URL   string `json:"url"`
		} `json:"proxy"`
	} `json:"info"`
	Description string `json:"description"`
	FileCount   int    `json:"file_count"`
	Error       struct {
		Kind string `json:"kind"`
		Msg  string `json:"msg"`
	} `json:"error"`
}
