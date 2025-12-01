package coreplugin

import (
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/consts"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func ParseProjectWithAutoDetective(ctx context.Context, path, language string, compileImmediately bool, inputs ...map[string]any) (*autoDetectInfo, *ssaapi.Program, func(), error) {
	pluginName := "SSA 项目探测"
	param := make(map[string]string)
	param["target"] = path
	param["language"] = language
	if compileImmediately {
		param["compile-immediately"] = "true"
	}
	for _, input := range inputs {
		for key, value := range input {
			param[key] = codec.AnyToString(value)
		}
	}

	var info *autoDetectInfo
	var err error

	// Step 1: 执行项目探测
	err = yakgrpc.ExecScriptWithParam(ctx, pluginName, param,
		"", func(exec *ypb.ExecResult) error {
			if !exec.IsMessage {
				return nil
			}
			rawMsg := exec.GetMessage()
			var msg msg
			json.Unmarshal(rawMsg, &msg)
			if msg.Type == "log" && msg.Content.Level == "code" {
				// Parse detective result
				err = json.Unmarshal([]byte(msg.Content.Data), &info)
				if err != nil {
					return err
				}
			}
			return nil
		},
	)
	if err != nil {
		return info, nil, nil, err
	}
	if info == nil {
		return nil, nil, nil, utils.Errorf("auto detective info is nil")
	}

	// Check for errors
	switch info.Error.Kind {
	case "languageNeedSelectException":
		return info, nil, nil, utils.Errorf("language need select")
	case "fileNotFoundException":
		return info, nil, nil, utils.Errorf("file not found")
	case "fileTypeException":
		return info, nil, nil, utils.Errorf("input file type")
	case "connectFailException":
		return info, nil, nil, utils.Errorf("connect fail")
	}

	// Step 2: 如果不需要立即编译，直接返回探测结果
	if !compileImmediately {
		return info, nil, nil, nil
	}

	// Step 3: 需要立即编译，创建项目并调用编译脚本
	config := info.Config
	if config == nil {
		return info, nil, nil, utils.Errorf("config is nil")
	}

	configJSON, err := config.ToJSONString()
	if err != nil {
		return info, nil, nil, utils.Errorf("failed to convert config to json: %s", err)
	}

	createReq := &ypb.CreateSSAProjectRequest{
		JSONStringConfig: configJSON,
	}

	profileDb := consts.GetGormProfileDatabase()
	createResp, err := yakit.CreateSSAProject(profileDb, createReq)
	if err != nil {
		return info, nil, nil, utils.Errorf("failed to create ssa project: %s", err)
	}

	cleanup := func() {
		deleteReq := &ypb.DeleteSSAProjectRequest{
			DeleteMode: string(yakit.SSAProjectDeleteAll),
			Filter: &ypb.SSAProjectFilter{
				IDs: []int64{int64(createResp.ID)},
			},
		}
		yakit.DeleteSSAProject(profileDb, deleteReq)
	}

	// 调用编译脚本
	projectConfig, err := createResp.GetConfig()
	if err != nil {
		return info, nil, nil, err
	}
	compilePluginName := "SSA 项目编译"
	compileParam := make(map[string]string)
	jsonString, _ := projectConfig.ToJSONString()
	compileParam["config"] = jsonString

	var compiledProgramName string
	err = yakgrpc.ExecScriptWithParam(ctx, compilePluginName, compileParam,
		"", func(exec *ypb.ExecResult) error {
			if !exec.IsMessage {
				return nil
			}
			rawMsg := exec.GetMessage()
			var msg msg
			json.Unmarshal(rawMsg, &msg)
			if msg.Type == "log" && msg.Content.Level == "code" {
				var result struct {
					ProgramName string `json:"program_name"`
				}
				err := json.Unmarshal([]byte(msg.Content.Data), &result)
				if err == nil && result.ProgramName != "" {
					compiledProgramName = result.ProgramName
				}
			}
			return nil
		},
	)
	if err != nil {
		return info, nil, cleanup, utils.Errorf("failed to compile project: %s", err)
	}

	if compiledProgramName == "" {
		return info, nil, cleanup, utils.Errorf("compiled program name is empty")
	}

	prog, err := ssaapi.FromDatabase(compiledProgramName)
	if err != nil {
		return info, nil, cleanup, utils.Errorf("failed to load program: %s", err)
	}

	return info, prog, cleanup, nil
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
	ProjectExists      bool `json:"project_exists"` // 项目是否已存在
	Error              struct {
		Kind string `json:"kind"`
		Msg  string `json:"msg"`
	} `json:"error"`
}
