package ssa_compile

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ParseProjectWithAutoDetective 调「SSA 项目探测」再可选调「SSA 项目编译」，返回探测信息与（若编译）Program。
// 插件由 coreplugin 注册，本包仅负责执行与结果组装。
func ParseProjectWithAutoDetective(ctx context.Context, conf *SSADetectConfig) (*SSADetectResult, error) {
	if conf == nil {
		conf = &SSADetectConfig{}
	}

	info, compileConfig, err := resolveCompileConfig(ctx, conf)
	if err != nil {
		return nil, err
	}

	if !conf.CompileImmediately {
		return &SSADetectResult{Info: info}, nil
	}

	prog, err := compileProject(ctx, compileConfig, conf.ForceProgramName, conf.DisableTimestampProgramName)
	if err != nil {
		return &SSADetectResult{Info: info}, err
	}

	return &SSADetectResult{
		Info:    info,
		Program: prog,
	}, nil
}

func resolveCompileConfig(ctx context.Context, conf *SSADetectConfig) (*AutoDetectInfo, *ssaconfig.Config, error) {
	if conf.Config != nil {
		cfg, err := cloneConfigWithOptions(conf.Config, append(conf.Options, ssaconfig.WithContext(ctx))...)
		if err != nil {
			return nil, nil, err
		}
		return &AutoDetectInfo{
			Config:             cfg,
			CompileImmediately: conf.CompileImmediately,
		}, cfg, nil
	}

	if strings.TrimSpace(conf.Target) == "" {
		return nil, nil, utils.Errorf("target is required when config is empty")
	}

	info, err := detectProject(ctx, conf.Target, conf.Language)
	if err != nil {
		return nil, nil, err
	}
	if info == nil || info.Config == nil {
		return nil, nil, utils.Errorf("auto detective config is nil")
	}

	cfg, err := cloneConfigWithOptions(info.Config, append(conf.Options, ssaconfig.WithContext(ctx))...)
	if err != nil {
		return nil, nil, err
	}
	info.Config = cfg
	info.CompileImmediately = conf.CompileImmediately
	return info, cfg, nil
}

func cloneConfigWithOptions(base *ssaconfig.Config, options ...ssaconfig.Option) (*ssaconfig.Config, error) {
	if base == nil {
		return nil, utils.Errorf("base config is nil")
	}
	raw, err := base.ToJSONRaw()
	if err != nil {
		return nil, utils.Errorf("marshal config failed: %s", err)
	}
	cfg, err := ssaconfig.NewCLIScanConfig(ssaconfig.WithJsonRawConfig(raw))
	if err != nil {
		return nil, utils.Errorf("clone config failed: %s", err)
	}
	if err := cfg.Update(options...); err != nil {
		return nil, utils.Errorf("apply config options failed: %s", err)
	}
	return cfg, nil
}

func detectProject(ctx context.Context, target, language string) (*AutoDetectInfo, error) {
	pluginName := "SSA 项目探测"
	param := map[string]string{
		"target": target,
	}
	if strings.TrimSpace(language) != "" {
		param["language"] = language
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

func compileProject(ctx context.Context, config *ssaconfig.Config, forceProgramName, disableTimestampProgramName bool) (*ssaapi.Program, error) {
	if config == nil {
		return nil, utils.Errorf("config is nil")
	}

	if shouldCompileInMemory(config) {
		configJSON, err := config.ToJSONString()
		if err != nil {
			return nil, utils.Errorf("failed to convert config to json: %s", err)
		}
		progs, err := ssaapi.ParseProject(
			ssaconfig.WithConfigJson(configJSON),
			ssaconfig.WithContext(ctx),
		)
		if err != nil {
			return nil, utils.Errorf("failed to compile project (memory): %s", err)
		}
		if len(progs) == 0 {
			return nil, utils.Errorf("compile project (memory) returned no programs")
		}
		return progs[0], nil
	}

	compiledProgramName, err := compileProjectByPlugin(ctx, config, forceProgramName, disableTimestampProgramName)
	if err != nil {
		return nil, err
	}

	if compiledProgramName == "" {
		compiledProgramName = config.GetProgramName()
	}
	if compiledProgramName == "" {
		return nil, utils.Errorf("compiled program name is empty")
	}

	return loadProgramWithRetry(compiledProgramName)
}

func compileProjectByPlugin(ctx context.Context, config *ssaconfig.Config, forceProgramName, disableTimestampProgramName bool) (string, error) {
	compilePluginName := "SSA 项目编译"
	configJSON, err := config.ToJSONString()
	if err != nil {
		return "", utils.Errorf("failed to convert config to json: %s", err)
	}
	compileParam := map[string]string{
		"config": configJSON,
	}
	if forceProgramName {
		if programName := strings.TrimSpace(config.GetProgramName()); programName != "" {
			compileParam["program_name"] = programName
		}
	}
	if disableTimestampProgramName {
		compileParam["disable_timestamp_program_name"] = "true"
		if _, hasProgramName := compileParam["program_name"]; !hasProgramName {
			if projectName := strings.TrimSpace(config.GetProjectName()); projectName != "" {
				compileParam["program_name"] = projectName
			}
		}
	}

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
		return "", utils.Errorf("failed to compile project: %s", err)
	}
	return compiledProgramName, nil
}

func loadProgramWithRetry(programName string) (*ssaapi.Program, error) {
	var (
		prog *ssaapi.Program
		err  error
	)
	maxRetries := 10
	retryDelay := 100 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		prog, err = ssaapi.FromDatabase(programName)
		if err == nil {
			return prog, nil
		}
		if i < maxRetries-1 {
			log.Debugf("program %s not found in database, retrying... (attempt %d/%d)", programName, i+1, maxRetries)
			time.Sleep(retryDelay)
			retryDelay *= 2
		}
	}
	return nil, utils.Errorf("failed to load program after %d retries: %s", maxRetries, err)
}

func shouldCompileInMemory(config *ssaconfig.Config) bool {
	if config == nil {
		return false
	}
	return config.GetCompileMemory()
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
