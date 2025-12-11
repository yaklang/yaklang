package yakcmds

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/coreplugin"
	"github.com/yaklang/yaklang/common/log"
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

func parseSFScanConfig(c *cli.Context) (res *ssaCliConfig, err error) {
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

	// Check if config file is provided
	configFilePath := c.String("config")
	if configFilePath != "" {
		log.Infof("Loading scan configuration from file: %s", configFilePath)
		data, err := os.ReadFile(configFilePath)
		if err != nil {
			return nil, utils.Errorf("failed to read config file %s: %v", configFilePath, err)
		}
		// 从JSON加载配置
		opts = append(opts, ssaconfig.WithJsonRawConfig(data))
	}

	// 命令行参数优先级高于配置文件

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
		opts = append(opts, ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal))
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
		excludeFiles := strings.Split(excludeFileStr, ",")
		var validExcludeFiles []string
		for _, s := range excludeFiles {
			if s != "" {
				validExcludeFiles = append(validExcludeFiles, strings.TrimSpace(s))
			}
		}
		if len(validExcludeFiles) > 0 {
			opts = append(opts, ssaconfig.WithCompileExcludeFiles(validExcludeFiles))
		}
	}

	// with-file-content
	if c.Bool("with-file-content") {
		opts = append(opts, ssaconfig.WithOutputFileContent(true))
	}

	// with-dataflow-path
	if c.Bool("with-dataflow-path") {
		opts = append(opts, ssaconfig.WithOutputDataflowPath(true))
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
		_, prog, _, err := coreplugin.ParseProjectWithAutoDetective(ctx, targetPath, language, true, para)
		return []*ssaapi.Program{prog}, err
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
