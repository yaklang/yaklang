package yakcmds

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa_compile"
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
	// preferConfigCompile 为 true 时，编译阶段直接使用 Config 走 script 编译链路。
	// 为 false 时，优先走 target + detect + script 链路。
	preferConfigCompile bool
	// forceProgramName 为 true 时，编译阶段显式固定 program_name（例如 CLI 指定 --program）。
	forceProgramName bool

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
		opts = append(opts, ssaconfig.WithSetProgramName(programName))
	}

	// target path -> code source
	if targetPath := c.String("target"); targetPath != "" {
		opts = append(opts, ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal))
		opts = append(opts, ssaconfig.WithCodeSourceLocalFile(targetPath))
	}

	// Language
	opts = append(opts, ssaconfig.WithProjectRawLanguage(c.String("language")))

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
		Config:              cfg,
		preferConfigCompile: false,
		forceProgramName:    c.IsSet("program") && strings.TrimSpace(c.String("program")) != "",
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

func logCompileStageMessage(command string, config *ssaCliConfig) {
	if config == nil || config.Config == nil {
		return
	}
	targetPath := config.GetCodeSourceLocalFileOrURL()
	programName := config.GetProgramName()
	language := config.GetLanguage()

	if targetPath != "" {
		pipeline := "ParseProjectWithAutoDetective(target + detect)"
		if config.preferConfigCompile {
			pipeline = "ParseProjectWithAutoDetective(config direct)"
		}
		log.Infof("[%s] compile stage: target=%q program=%q language=%q pipeline=%s",
			command, targetPath, programName, language, pipeline)
	} else {
		log.Infof("[%s] compile stage: load existing program from database by pattern=%q", command, programName)
	}

	log.Infof(
		"[%s] compile options: re-compile=%v entry-files=%d exclude-files=%d file-perf-log=%v compile-memory=%v scan-memory=%v",
		command,
		config.GetCompileReCompile(),
		len(config.GetCompileEntryFiles()),
		len(config.GetCompileExcludeFiles()),
		config.GetCompileFilePerformanceLog(),
		config.GetCompileMemory(),
		config.GetSyntaxFlowMemory(),
	)
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

	if targetPath != "" {
		if config.preferConfigCompile {
			log.Infof("get program from target path: %s (config-direct compile mode)", targetPath)
		} else {
			log.Infof("get program from target path: %s (target-detect compile mode)", targetPath)
		}
		req := &ssa_compile.SSADetectConfig{
			CompileImmediately:          true,
			ForceProgramName:            config.forceProgramName,
			DisableTimestampProgramName: true,
		}
		if config.preferConfigCompile {
			req.Config = config.Config
		} else {
			req.Target = targetPath
			req.Language = string(config.GetLanguage())
			req.Options = buildCompileOptionsForDetect(config.Config)
		}

		res, err := ssa_compile.ParseProjectWithAutoDetective(ctx, req)
		if err != nil {
			return nil, err
		}
		if res == nil || res.Program == nil {
			return nil, utils.Errorf("compile result is empty")
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
	if err := applyCompileCliOverrides(cfg, cliCtx); err != nil {
		return nil, err
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
		Config:              cfg,
		preferConfigCompile: true,
		forceProgramName:    strings.TrimSpace(cfg.GetProgramName()) != "",
	}

	// 设置输出格式
	outputFormat := cfg.GetOutputFormat()
	if outputFormat == "" {
		outputFormat = "sarif" // 默认格式
	}
	config.Format = sfreport.ReportTypeFromString(outputFormat)

	// 处理输出文件：CLI 参数优先于配置文件
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

func parseCompileConfigFromCli(c *cli.Context) (res *ssaCliConfig, err error) {
	log.Infof("================= parse compile config ================")
	defer func() {
		log.Infof("parse compile config done")
		if panicErr := recover(); panicErr != nil {
			log.Errorf("parse compile config failed: %s", panicErr)
			utils.PrintCurrentGoroutineRuntimeStack()
			err = utils.Errorf("parse compile config failed: %s", panicErr)
		}
	}()

	opts := []ssaconfig.Option{
		ssaconfig.WithProjectRawLanguage(c.String("language")),
	}
	if programName := c.String("program"); programName != "" {
		opts = append(opts, ssaconfig.WithSetProgramName(programName))
	}
	if targetPath := c.String("target"); targetPath != "" {
		opts = append(opts,
			ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
			ssaconfig.WithCodeSourceLocalFile(targetPath),
		)
	}
	if c.Bool("re-compile") {
		opts = append(opts, ssaconfig.WithCompileReCompile(true))
	}
	if excludeFileStr := c.String("exclude-file"); excludeFileStr != "" {
		opts = append(opts, ssaconfig.WithCompileExcludeFiles(excludeFileStr))
	}
	if entry := c.String("entry"); entry != "" {
		opts = append(opts, ssaconfig.WithCompileEntryFiles(entry))
	}
	if c.Bool("file-perf-log") {
		opts = append(opts, ssaconfig.WithCompileFilePerformanceLog(true))
	}

	cfg, err := ssaconfig.NewCLIScanConfig(opts...)
	if err != nil {
		return nil, utils.Errorf("failed to create compile config: %v", err)
	}
	if cfg.GetProgramName() == "" && cfg.GetCodeSourceLocalFileOrURL() == "" {
		return nil, utils.Errorf("either --program, --target, or --config must be specified")
	}

	return &ssaCliConfig{
		Config:              cfg,
		preferConfigCompile: false,
		forceProgramName:    c.IsSet("program") && strings.TrimSpace(c.String("program")) != "",
	}, nil
}

func buildCompileOptionsForDetect(cfg *ssaconfig.Config) []ssaconfig.Option {
	if cfg == nil {
		return nil
	}
	opts := make([]ssaconfig.Option, 0, 10)
	if programName := cfg.GetProgramName(); programName != "" {
		opts = append(opts, ssaconfig.WithSetProgramName(programName))
	}
	opts = append(opts, ssaconfig.WithProjectRawLanguage(string(cfg.GetLanguage())))
	if cfg.GetCompileMemory() || cfg.GetSyntaxFlowMemory() {
		opts = append(opts, ssaconfig.WithCompileMemoryCompile(true))
	}
	if excludes := cfg.GetCompileExcludeFiles(); len(excludes) > 0 {
		opts = append(opts, ssaconfig.WithCompileExcludeFiles(excludes...))
	}
	if entries := cfg.GetCompileEntryFiles(); len(entries) > 0 {
		opts = append(opts, ssaconfig.WithCompileEntryFiles(entries...))
	}
	if cfg.GetCompileReCompile() {
		opts = append(opts, ssaconfig.WithCompileReCompile(true))
	}
	if cfg.GetCompileFilePerformanceLog() {
		opts = append(opts, ssaconfig.WithCompileFilePerformanceLog(true))
	}
	if cfg.GetCompileStrictMode() {
		opts = append(opts, ssaconfig.WithCompileStrictMode(true))
	}
	if concurrency := cfg.GetCompileConcurrency(); concurrency > 0 {
		opts = append(opts, ssaconfig.WithCompileConcurrency(concurrency))
	}
	return opts
}

func applyCompileCliOverrides(cfg *ssaconfig.Config, cliCtx *cli.Context) error {
	if cfg == nil {
		return utils.Errorf("config is nil")
	}
	if cliCtx == nil {
		return nil
	}

	opts := make([]ssaconfig.Option, 0, 16)
	if cliCtx.IsSet("program") {
		if programName := strings.TrimSpace(cliCtx.String("program")); programName != "" {
			opts = append(opts, ssaconfig.WithSetProgramName(programName))
		}
	}
	if cliCtx.IsSet("target") {
		if targetPath := strings.TrimSpace(cliCtx.String("target")); targetPath != "" {
			opts = append(opts,
				ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
				ssaconfig.WithCodeSourceLocalFile(targetPath),
				ssaconfig.WithCodeSourceURL(""),
			)
		}
	}
	if cliCtx.IsSet("language") {
		opts = append(opts, ssaconfig.WithProjectRawLanguage(cliCtx.String("language")))
	}
	if cliCtx.IsSet("memory") {
		enable := cliCtx.Bool("memory")
		opts = append(opts,
			ssaconfig.WithCompileMemoryCompile(enable),
			ssaconfig.WithSyntaxFlowMemory(enable),
		)
	}
	if cliCtx.IsSet("file-perf-log") {
		opts = append(opts, ssaconfig.WithCompileFilePerformanceLog(cliCtx.Bool("file-perf-log")))
	}
	if cliCtx.IsSet("re-compile") {
		opts = append(opts, ssaconfig.WithCompileReCompile(cliCtx.Bool("re-compile")))
	}
	if cliCtx.IsSet("format") {
		opts = append(opts, ssaconfig.WithOutputFormat(cliCtx.String("format")))
	}
	if cliCtx.IsSet("output") {
		opts = append(opts, ssaconfig.WithOutputFile(cliCtx.String("output")))
	}
	if cliCtx.IsSet("with-file-content") {
		opts = append(opts, ssaconfig.WithOutputFileContent(cliCtx.Bool("with-file-content")))
	}
	if cliCtx.IsSet("with-dataflow-path") {
		opts = append(opts, ssaconfig.WithOutputDataflowPath(cliCtx.Bool("with-dataflow-path")))
	}
	if err := cfg.Update(opts...); err != nil {
		return utils.Errorf("apply cli overrides failed: %v", err)
	}

	if cliCtx.IsSet("exclude-file") {
		excludeFileStr := strings.TrimSpace(cliCtx.String("exclude-file"))
		if excludeFileStr == "" {
			cfg.SetCompileExcludeFiles(nil)
		} else {
			cfg.SetCompileExcludeFiles([]string{excludeFileStr})
		}
	}
	if cliCtx.IsSet("entry") {
		entry := strings.TrimSpace(cliCtx.String("entry"))
		if entry == "" {
			cfg.SetCompileEntryFiles(nil)
		} else {
			cfg.SetCompileEntryFiles([]string{entry})
		}
	}
	return nil
}
