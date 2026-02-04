package ssa_compile

import (
	"context"
	"encoding/json"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ParseProjectWithAutoDetective 调「SSA 项目探测」再可选调「SSA 项目编译」，返回探测信息与（若编译）Program。
// 插件由 coreplugin 注册，本包仅负责执行与结果组装。
func ParseProjectWithAutoDetective(ctx context.Context, conf *SSADetectConfig) (*SSADetectResult, error) {
	info, err := detectProject(ctx, conf)
	if err != nil {
		return nil, err
	}

	if !conf.CompileImmediately {
		return &SSADetectResult{Info: info}, nil
	}

	prog, cleanup, err := compileProject(ctx, info)
	if err != nil {
		return &SSADetectResult{Info: info}, err
	}

	return &SSADetectResult{
		Info:    info,
		Program: prog,
		Cleanup: cleanup,
	}, nil
}

func detectProject(ctx context.Context, conf *SSADetectConfig) (*AutoDetectInfo, error) {
	pluginName := "SSA 项目探测"
	param := make(map[string]string)
	param["target"] = conf.Target
	param["language"] = conf.Language
	if conf.CompileImmediately {
		param["compile-immediately"] = "true"
	}
	for key, value := range conf.Params {
		param[key] = codec.AnyToString(value)
	}

	var info *AutoDetectInfo
	err := yakgrpc.ExecScriptWithParam(ctx, pluginName, param,
		"", func(exec *ypb.ExecResult) error {
			if !exec.IsMessage {
				return nil
			}
			rawMsg := exec.GetMessage()
			var msg execMsg
			json.Unmarshal(rawMsg, &msg)
			if msg.Type == "log" && msg.Content.Level == "code" {
				err := json.Unmarshal([]byte(msg.Content.Data), &info)
				if err != nil {
					return err
				}
			}
			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, utils.Errorf("auto detective info is nil")
	}

	switch info.Error.Kind {
	case "languageNeedSelectException":
		return info, utils.Errorf("language need select")
	case "fileNotFoundException":
		return info, utils.Errorf("file not found")
	case "fileTypeException":
		return info, utils.Errorf("input file type")
	case "connectFailException":
		return info, utils.Errorf("connect fail")
	}
	return info, nil
}

func compileProject(ctx context.Context, info *AutoDetectInfo) (*ssaapi.Program, func(), error) {
	config := info.Config
	if config == nil {
		return nil, nil, utils.Errorf("config is nil")
	}

	configJSON, err := config.ToJSONString()
	if err != nil {
		return nil, nil, utils.Errorf("failed to convert config to json: %s", err)
	}

	createReq := &ypb.CreateSSAProjectRequest{
		JSONStringConfig: configJSON,
	}

	profileDb := consts.GetGormProfileDatabase()
	createResp, err := yakit.CreateSSAProject(profileDb, createReq)
	if err != nil {
		return nil, nil, utils.Errorf("failed to create ssa project: %s", err)
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

	projectConfig, err := createResp.GetConfig()
	if err != nil {
		return nil, cleanup, err
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
			var msg execMsg
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
		return nil, cleanup, utils.Errorf("failed to compile project: %s", err)
	}

	if compiledProgramName == "" {
		return nil, cleanup, utils.Errorf("compiled program name is empty")
	}

	var prog *ssaapi.Program
	maxRetries := 10
	retryDelay := 100 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		prog, err = ssaapi.FromDatabase(compiledProgramName)
		if err == nil {
			break
		}
		if i < maxRetries-1 {
			log.Debugf("program %s not found in database, retrying... (attempt %d/%d)", compiledProgramName, i+1, maxRetries)
			time.Sleep(retryDelay)
			retryDelay *= 2
		}
	}

	if err != nil {
		return nil, cleanup, utils.Errorf("failed to load program after %d retries: %s", maxRetries, err)
	}

	return prog, cleanup, nil
}

type execMsg struct {
	Type    string `json:"type"`
	Content struct {
		Level   string  `json:"level"`
		Data    string  `json:"data"`
		ID      string  `json:"id"`
		Process float64 `json:"progress"`
	}
}
