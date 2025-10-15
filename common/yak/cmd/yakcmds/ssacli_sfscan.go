package yakcmds

import (
	"context"
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
	sync := false
	if len(force) > 0 {
		sync = force[0]
	}
	log.Infof("================= check builtin rule sync ================")
	if sync {
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
	// Parse and validate output configuration
	config := &ssaCliConfig{}
	// 	OutputFile:  writer,
	// 	Format:      format,
	// 	programName: programName,
	// 	targetPath:  targetPath,
	// }

	config.Format = sfreport.ReportTypeFromString(c.String("format"))

	// Parse program configuration
	programName := c.String("program")
	targetPath := c.String("target")
	if programName == "" && targetPath == "" {
		return nil, utils.Errorf("either --program or --target must be specified")
	} else {
		config.programName = programName
		config.targetPath = targetPath
	}
	config.language = c.String("language")
	config.memory = c.Bool("memory")

	// result  writer
	// var writer io.Writer
	outputFile := c.String("output")
	if outputFile == "" {
		log.Infof("output file is not specified, use stdout")
		// writer = os.Stdout
		config.OutputWriter = os.Stdout
	} else {
		if config.Format == sfreport.SarifReportType {
			if filepath.Ext(outputFile) != ".sarif" {
				outputFile += ".sarif"
			}
		} else {
			if filepath.Ext(outputFile) != ".json" {
				outputFile += ".json"
			}
		}
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

	if e := c.String("exclude-file"); e != "" {
		config.exclude = e
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
	if config.programName != "" {
		log.Infof("get program from database: %s", config.programName)
		return ssaapi.FromDatabase(config.programName)
	}
	if config.targetPath != "" {
		log.Infof("get program from target path: %s", config.targetPath)
		para := make(map[string]any)
		if config.memory {
			para["memory"] = true
			para["excludeFile"] = config.exclude
		}
		_, prog, err := coreplugin.ParseProjectWithAutoDetective(ctx, config.targetPath, config.language, para)
		return prog, err
	}
	return nil, utils.Errorf("get program by parameter fail, please check your command")
}
