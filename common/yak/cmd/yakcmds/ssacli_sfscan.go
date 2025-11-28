package yakcmds

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/coreplugin"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
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
	// {{ parse program
	// programName is the name of the program
	programName string
	// targetPath is the path of the target
	targetPath string

	language string
	memory   bool
	// }}

	// {{ should result
	// OutputWriter is the file to save the result
	OutputWriter io.Writer
	// Format is the format of the result
	Format sfreport.ReportType // sarif or json
	// }}

	// {{ defer function
	deferFunc []func()
	// }}

	exclude string
}

// ScanConfigFile represents the JSON configuration file for code-scan
type ScanConfigFile struct {
	ProjectName     string            `json:"project_name"`
	Language        string            `json:"language"`
	CodeSource      *CodeSourceConfig `json:"code_source"`
	ExcludePatterns []string          `json:"exclude_patterns"`
	RuleNames       []string          `json:"rule_names"`
	Memory          bool              `json:"memory"`
	OutputFile      string            `json:"output_file"`
	OutputFormat    string            `json:"output_format"`
}

type CodeSourceConfig struct {
	Kind   string `json:"kind"`   // "git" or "local"
	URL    string `json:"url"`    // for git
	Branch string `json:"branch"` // for git
	Path   string `json:"path"`   // for local
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

	config := &ssaCliConfig{}

	// Check if config file is provided
	configFilePath := c.String("config")
	var fileConfig *ScanConfigFile

	if configFilePath != "" {
		log.Infof("Loading scan configuration from file: %s", configFilePath)
		data, err := os.ReadFile(configFilePath)
		if err != nil {
			return nil, utils.Errorf("failed to read config file %s: %v", configFilePath, err)
		}

		fileConfig = &ScanConfigFile{}
		if err := json.Unmarshal(data, fileConfig); err != nil {
			return nil, utils.Errorf("failed to parse config file %s: %v", configFilePath, err)
		}

		log.Infof("Config file loaded: project=%s, language=%s",
			fileConfig.ProjectName, fileConfig.Language)
	}

	// Merge configuration: command line flags take precedence over config file

	// Output format
	format := c.String("format")
	if format == "" && fileConfig != nil && fileConfig.OutputFormat != "" {
		format = fileConfig.OutputFormat
	}
	config.Format = sfreport.ReportTypeFromString(format)

	// Parse program configuration
	programName := c.String("program")
	targetPath := c.String("target")

	// If not provided via command line, use config file values
	// Priority: targetPath > programName (code_source takes precedence over database lookup)
	if targetPath == "" && fileConfig != nil && fileConfig.CodeSource != nil {
		if fileConfig.CodeSource.Kind == "local" {
			targetPath = fileConfig.CodeSource.Path
		} else if fileConfig.CodeSource.Kind == "git" {
			targetPath = fileConfig.CodeSource.URL
		}
	}
	if programName == "" && fileConfig != nil {
		programName = fileConfig.ProjectName
	}

	if programName == "" && targetPath == "" {
		return nil, utils.Errorf("either --program, --target, or --config with valid code_source must be specified")
	}

	config.programName = programName
	config.targetPath = targetPath

	// Language
	language := c.String("language")
	if language == "" && fileConfig != nil {
		language = fileConfig.Language
	}
	config.language = language

	// Memory mode
	memory := c.Bool("memory")
	if !memory && fileConfig != nil {
		memory = fileConfig.Memory
	}
	config.memory = memory

	// Output file
	outputFile := c.String("output")
	if outputFile == "" && fileConfig != nil {
		outputFile = fileConfig.OutputFile
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

	// Exclude patterns
	excludeFile := c.String("exclude-file")
	if excludeFile == "" && fileConfig != nil && len(fileConfig.ExcludePatterns) > 0 {
		// Join multiple patterns (the exclude-file flag can accept comma-separated patterns)
		excludeFile = ""
		for i, pattern := range fileConfig.ExcludePatterns {
			if i > 0 {
				excludeFile += ","
			}
			excludeFile += pattern
		}
	}
	if excludeFile != "" {
		config.exclude = excludeFile
	}

	return config, nil
}

// getProgram gets the program using the provided configuration
func getProgram(ctx context.Context, config *ssaCliConfig) (*ssaapi.Program, error) {
	log.Infof("================= get or parse program ================")
	defer func() {
		log.Infof("get program done")
		if err := recover(); err != nil {
			log.Errorf("get program failed: %s", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	// Priority: targetPath > programName (compile from source > database lookup)
	if config.targetPath != "" {
		log.Infof("get program from target path: %s", config.targetPath)
		para := make(map[string]any)
		if config.memory {
			para["memory"] = true
		}
		if config.exclude != "" {
			para["excludeFile"] = config.exclude
		}
		if config.programName != "" {
			para["program_name"] = config.programName
		}
		_, prog, _, err := coreplugin.ParseProjectWithAutoDetective(ctx, config.targetPath, config.language, true, para)
		return prog, err
	}
	if config.programName != "" {
		log.Infof("get program from database: %s", config.programName)
		return ssaapi.FromDatabase(config.programName)
	}
	return nil, utils.Errorf("get program by parameter fail, please check your command")
}
