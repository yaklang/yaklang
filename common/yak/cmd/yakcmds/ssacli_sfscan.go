package yakcmds

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa_compile"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func SyncEmbedRule(force ...bool) {
	isForce := false
	if len(force) > 0 {
		isForce = force[0]
	}
	log.Infof("================= check builtin rule sync ================")
	if isForce {
		// 强制同步：用于命令行 sync-rule，忽略哈希检查
		sfbuildin.ForceSyncEmbedRule(func(process float64, ruleName string) {
			log.Infof("force sync embed rule: %s, process: %f", ruleName, process)
		})
	} else {
		// 自动同步：用于应用启动，检查哈希
		sfbuildin.SyncEmbedRule(func(process float64, ruleName string) {
			log.Infof("sync embed rule: %s, process: %f", ruleName, process)
		})
	}
}

type ssaCliConfig struct {
	// 统一配置
	*ssaconfig.Config

	// {{ should result
	// OutputWriter is the file to save the result
	OutputWriter io.Writer
	// Format is the format of the result
	Format sfreport.ReportType // sarif or json
	// }}

	// {{ defer function
	deferFunc []func()
	// }}
}

func (config *ssaCliConfig) DeferFunc() {
	for _, f := range config.deferFunc {
		f()
	}
}

func parseSFScanConfigFromCli(c *cli.Context) (res *ssaCliConfig, err error) {
	log.Infof("================= parse config ================")
	defer func() {
		log.Infof("parse config done")
		if err := recover(); err != nil {
			log.Errorf("parse config failed: %s", err)
			utils.PrintCurrentGoroutineRuntimeStack()
			err = utils.Errorf("parse config failed: %s", err)
		}
	}()

	// 收集所有配置选项
	var opts []ssaconfig.Option

	// Output format
	if format := c.String("format"); format != "" {
		opts = append(opts, ssaconfig.WithOutputFormat(format))
	}

	// Parse program configuration
	if programName := c.String("program"); programName != "" {
		opts = append(opts, ssaconfig.WithProgramNames(programName))
	}

	// target path -> code source
	if targetPath := c.String("target"); targetPath != "" {
		opts = append(opts, ssaconfig.WithCodeSourceLocalFile(targetPath))
	}

	// Language
	if language := c.String("language"); language != "" {
		opts = append(opts, ssaconfig.WithProjectRawLanguage(language))
	}

	// Memory mode
	if c.Bool("memory") {
		opts = append(opts, ssaconfig.WithCompileMemoryCompile(true))
		opts = append(opts, ssaconfig.WithSyntaxFlowMemory(true))
	}

	// Output file
	if outputFile := c.String("output"); outputFile != "" {
		opts = append(opts, ssaconfig.WithOutputFile(outputFile))
	}

	// Exclude patterns
	if excludeFileStr := c.String("exclude-file"); excludeFileStr != "" {
		opts = append(opts, ssaconfig.WithCompileExcludeFiles(excludeFileStr))
	}

	// with-file-content
	if c.Bool("with-file-content") {
		opts = append(opts, ssaconfig.WithOutputFileContent(true))
	}

	// with-dataflow-path
	if c.Bool("with-dataflow-path") {
		opts = append(opts, ssaconfig.WithOutputDataflowPath(true))
	}

	// file-perf-log: 启用文件级别性能日志
	if c.Bool("file-perf-log") {
		opts = append(opts, ssaconfig.WithCompileFilePerformanceLog(true))
	}

	// 创建统一配置
	cfg, err := ssaconfig.NewCLIScanConfig(opts...)
	if err != nil {
		return nil, utils.Errorf("failed to create config: %v", err)
	}

	// 调试日志
	log.Infof("Config loaded: programName=%s, language=%s, targetPath=%s, outputFile=%s, outputFormat=%s",
		cfg.GetProgramName(), cfg.GetLanguage(), cfg.GetCodeSourceLocalFileOrURL(),
		cfg.GetOutputFile(), cfg.GetOutputFormat())

	// 验证必要配置
	programName := cfg.GetProgramName()
	targetPath := cfg.GetCodeSourceLocalFileOrURL()

	if programName == "" && targetPath == "" {
		return nil, utils.Errorf("either --program, --target, or --config with valid code_source must be specified")
	}

	config := &ssaCliConfig{
		Config: cfg,
	}

	// 设置输出格式
	outputFormat := cfg.GetOutputFormat()
	if outputFormat == "" {
		outputFormat = "sarif" // 默认格式
	}
	config.Format = sfreport.ReportTypeFromString(outputFormat)

	// 处理输出文件
	outputFile := cfg.GetOutputFile()
	if outputFile == "" {
		log.Infof("output file is not specified, use stdout")
		config.OutputWriter = os.Stdout
	} else {
		// Add appropriate file extension
		if config.Format == sfreport.SarifReportType {
			if filepath.Ext(outputFile) != ".sarif" {
				outputFile += ".sarif"
			}
		} else {
			if filepath.Ext(outputFile) != ".json" {
				outputFile += ".json"
			}
		}

		// Backup existing file
		if utils.GetFirstExistedFile(outputFile) != "" {
			backup := outputFile + ".bak"
			os.Rename(outputFile, backup)
			os.RemoveAll(outputFile)
		}

		file, err := os.OpenFile(outputFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return nil, utils.Errorf("failed to create output file: %v", err)
		}
		config.OutputWriter = file
		config.deferFunc = append(config.deferFunc, func() {
			file.Close()
		})
	}

	return config, nil
}

// getProgram gets the program using the provided configuration
func getProgram(ctx context.Context, config *ssaCliConfig) ([]*ssaapi.Program, error) {
	log.Infof("================= get or parse program ================")
	defer func() {
		log.Infof("get program done")
		if err := recover(); err != nil {
			log.Errorf("get program failed: %s", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	targetPath := config.GetCodeSourceLocalFileOrURL()
	programName := config.GetProgramName()
	language := string(config.GetLanguage())
	memory := config.GetCompileMemory() || config.GetSyntaxFlowMemory()
	excludeFiles := strings.Join(config.GetCompileExcludeFiles(), ",")

	if targetPath != "" {
		log.Infof("get program from target path: %s", targetPath)
		para := make(map[string]any)
		if memory {
			para["memory"] = true
		}
		if len(excludeFiles) > 0 {
			para["excludeFile"] = excludeFiles
		}
		if programName != "" {
			para["program_name"] = programName
		}
		// 传递文件性能日志配置
		if config.GetCompileFilePerformanceLog() {
			para["filePerformanceLog"] = true
		}
		if memory {
			log.Infof("memory mode enabled, compile in-process to keep program in memory")
			res, err := ssa_compile.ParseProjectWithAutoDetective(ctx, &ssa_compile.SSADetectConfig{
				Target:             targetPath,
				Language:           language,
				CompileImmediately: false,
				Params:             para,
			})
			if err != nil {
				return nil, err
			}
			if res == nil || res.Info == nil || res.Info.Config == nil {
				return nil, utils.Errorf("auto detective config is nil in memory mode")
			}
			cfg := res.Info.Config
			cfg.SetCompileMemory(true)
			if programName != "" {
				cfg.SetProgramName(programName)
			}
			if len(config.GetCompileExcludeFiles()) > 0 {
				cfg.SetCompileExcludeFiles(config.GetCompileExcludeFiles())
			}
			if config.GetCompileFilePerformanceLog() {
				cfg.SetCompileFilePerformanceLog(true)
			}
			progs, err := ssaapi.ParseProject(
				ssaconfig.WithConfigJson(cfg.JSON()),
				ssaconfig.WithContext(ctx),
			)
			if err != nil {
				return nil, err
			}
			if len(progs) == 0 {
				return nil, utils.Errorf("compile project returned no programs (memory mode)")
			}
			return progs, nil
		}
		res, err := ssa_compile.ParseProjectWithAutoDetective(ctx, &ssa_compile.SSADetectConfig{
			Target:             targetPath,
			Language:           language,
			CompileImmediately: true,
			Params:             para,
		})
		if err != nil {
			return nil, err
		}
		return []*ssaapi.Program{res.Program}, nil
	}

	if programName != "" {
		log.Infof("get program from database: %s", programName)
		ret := ssaapi.LoadProgramRegexp(programName)
		if len(ret) == 0 {
			return nil, utils.Errorf("program %s not found in database", programName)
		}
		return ret, nil
	}

	return nil, utils.Errorf("get program by parameter fail, please check your command")
}

// parseConfigFileOnlyWithOutputOverride 从配置文件加载配置，并允许 CLI 参数覆盖
// cliOutputFile 为空时使用配置文件中的设置，非空时优先使用 CLI 参数
func parseConfigFileWithCliFlagOverride(cliCtx *cli.Context) (res *ssaCliConfig, err error) {
	configFilePath := cliCtx.String("config")
	cliOutputFile := cliCtx.String("output")

	log.Infof("================= parse config file ================")
	defer func() {
		log.Infof("parse config file done")
		if err := recover(); err != nil {
			log.Errorf("parse config file failed: %s", err)
			utils.PrintCurrentGoroutineRuntimeStack()
			err = utils.Errorf("parse config file failed: %s", err)
		}
	}()

	if configFilePath == "" {
		return nil, utils.Errorf("config file path is required")
	}

	log.Infof("Loading scan configuration from file: %s", configFilePath)
	data, err := os.ReadFile(configFilePath)
	if err != nil {
		return nil, utils.Errorf("failed to read config file %s: %v", configFilePath, err)
	}

	// 从JSON加载配置
	cfg, err := ssaconfig.NewCLIScanConfig(ssaconfig.WithJsonRawConfig(data))
	if err != nil {
		return nil, utils.Errorf("failed to create config from JSON: %v", err)
	}

	// 调试日志
	log.Infof("Config loaded: programName=%s, language=%s, targetPath=%s, outputFile=%s, outputFormat=%s",
		cfg.GetProgramName(), cfg.GetLanguage(), cfg.GetCodeSourceLocalFileOrURL(),
		cfg.GetOutputFile(), cfg.GetOutputFormat())

	// 验证必要配置
	programName := cfg.GetProgramName()
	targetPath := cfg.GetCodeSourceLocalFileOrURL()

	if programName == "" && targetPath == "" {
		return nil, utils.Errorf("config file must specify either program_names in BaseInfo or code_source with valid local_file/url")
	}

	config := &ssaCliConfig{
		Config: cfg,
	}

	// 设置输出格式
	outputFormat := cfg.GetOutputFormat()
	if outputFormat == "" {
		outputFormat = "sarif" // 默认格式
	}
	config.Format = sfreport.ReportTypeFromString(outputFormat)

	// 处理输出文件：CLI 参数优先于配置文件
	outputFile := cliOutputFile
	if outputFile != "" {
		log.Infof("Using CLI output file (overrides config): %s", outputFile)
	} else {
		outputFile = cfg.GetOutputFile()
	}

	if outputFile == "" {
		log.Infof("output file is not specified, use stdout")
		config.OutputWriter = os.Stdout
	} else {
		// Add appropriate file extension
		if config.Format == sfreport.SarifReportType {
			if filepath.Ext(outputFile) != ".sarif" {
				outputFile += ".sarif"
			}
		} else {
			if filepath.Ext(outputFile) != ".json" {
				outputFile += ".json"
			}
		}

		// Backup existing file
		if utils.GetFirstExistedFile(outputFile) != "" {
			backup := outputFile + ".bak"
			os.Rename(outputFile, backup)
			os.RemoveAll(outputFile)
		}

		file, err := os.OpenFile(outputFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return nil, utils.Errorf("failed to create output file: %v", err)
		}
		config.OutputWriter = file
		config.deferFunc = append(config.deferFunc, func() {
			file.Close()
		})
	}

	return config, nil
}

// getProgramForConfigScan 直接使用 ssaapi.ParseProject 编译程序 (for config-scan command)
// 不经过 coreplugin 的项目探测流程，减少与 code-scan 的链路耦合
func getProgramForConfigScan(ctx context.Context, config *ssaCliConfig) ([]*ssaapi.Program, error) {
	log.Infof("================= get or parse program (config-scan) ================")
	defer func() {
		log.Infof("get program done")
		if err := recover(); err != nil {
			log.Errorf("get program failed: %s", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	programName := config.GetProgramName()
	targetPath := config.GetCodeSourceLocalFileOrURL()

	// 逻辑说明：
	// 1. 如果指定了 CodeSource（targetPath 非空），则始终重新编译，不从数据库加载
	//    这意味着用户明确希望编译新的代码，无论数据库中是否存在同名程序
	// 2. 只有当 CodeSource 未指定（targetPath 为空）且指定了 programName 时，
	//    才尝试从数据库加载已有程序

	// 有 CodeSource → 始终编译
	if targetPath != "" {
		log.Infof("compiling program from target path: %s", targetPath)

		// 构建编译选项
		opts := []ssaconfig.Option{
			ssaconfig.WithContext(ctx),
		}

		// 设置程序名
		if programName != "" {
			opts = append(opts, ssaconfig.WithProgramNames(programName))
		}

		// 设置语言
		if lang := config.GetLanguage(); lang != "" {
			opts = append(opts, ssaconfig.WithProjectLanguage(lang))
		}

		// 设置 memory 模式（仅编译时的 memory）
		if config.GetCompileMemory() {
			opts = append(opts, ssaconfig.WithCompileMemoryCompile(true))
		}

		// 设置排除文件
		if excludeFiles := config.GetCompileExcludeFiles(); len(excludeFiles) > 0 {
			opts = append(opts, ssaconfig.WithCompileExcludeFiles(excludeFiles...))
		}

		// 设置重编译选项
		if config.GetCompileReCompile() {
			opts = append(opts, ssaconfig.WithCompileReCompile(true))
		}

		// 设置文件性能日志
		if config.GetCompileFilePerformanceLog() {
			opts = append(opts, ssaconfig.WithCompileFilePerformanceLog(true))
		}

		// 设置编译并发数
		if concurrency := config.GetCompileConcurrency(); concurrency > 0 {
			opts = append(opts, ssaconfig.WithCompileConcurrency(concurrency))
		}

		// 根据 CodeSource 类型设置文件系统
		codeSourceKind := config.GetCodeSourceKind()
		log.Infof("code source kind: %s, target path: %s", codeSourceKind, targetPath)

		switch codeSourceKind {
		case ssaconfig.CodeSourceLocal:
			opts = append(opts, ssaconfig.WithCodeSourceMap(map[string]any{
				"kind":       "local",
				"local_file": targetPath,
			}))
		case ssaconfig.CodeSourceGit:
			opts = append(opts, ssaconfig.WithCodeSourceMap(map[string]any{
				"kind":   "git",
				"url":    config.GetCodeSourceURL(),
				"branch": config.GetCodeSourceBranch(),
			}))
		case ssaconfig.CodeSourceCompression:
			opts = append(opts, ssaconfig.WithCodeSourceMap(map[string]any{
				"kind":       "compression",
				"local_file": targetPath,
			}))
		default:
			// 默认当作本地路径处理
			opts = append(opts, ssaconfig.WithCodeSourceMap(map[string]any{
				"kind":       "local",
				"local_file": targetPath,
			}))
		}

		// 调用 ssaapi.ParseProject 进行编译
		progs, err := ssaapi.ParseProject(opts...)
		if err != nil {
			return nil, utils.Errorf("compile project failed: %v", err)
		}
		if len(progs) == 0 {
			return nil, utils.Errorf("compile project returned no programs")
		}
		// 转换为 []*ssaapi.Program
		result := make([]*ssaapi.Program, 0, len(progs))
		for _, prog := range progs {
			result = append(result, prog)
		}
		return result, nil
	}

	// 没有 CodeSource，只有 programName → 从数据库加载已有程序
	if programName != "" {
		log.Infof("loading program from database: %s", programName)
		ret := ssaapi.LoadProgramRegexp(programName)
		if len(ret) == 0 {
			return nil, utils.Errorf("program %s not found in database", programName)
		}
		return ret, nil
	}

	return nil, utils.Errorf("config must specify either CodeSource (with local_file/url) or program_names in BaseInfo")
}
