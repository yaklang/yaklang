package yakcmds

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/coreplugin"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/sarif"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func SyncEmbedRule() {
	log.Infof("================= check builtin rule sync ================")
	if sfbuildin.CheckEmbedRule() {
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
	// }}

	// {{ should result
	// OutputWriter is the file to save the result
	OutputWriter io.Writer
	// Format is the format of the result
	Format string // sarif or json
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
	// Parse and validate output configuration
	config := &ssaCliConfig{}
	// 	OutputFile:  writer,
	// 	Format:      format,
	// 	programName: programName,
	// 	targetPath:  targetPath,
	// }

	// format
	// format := c.String("format")
	// if format == "" {
	// 	format = "sarif"
	// }
	// if format != "sarif" && format != "json" {
	// 	return nil, utils.Errorf("unsupported output format: %s, must be 'sarif' or 'json'", format)
	// } else {
	// 	config.Format = format
	// }
	if config.Format == "" {
		config.Format = "sarif"
	}

	// Parse program configuration
	programName := c.String("program")
	targetPath := c.String("target")
	if programName == "" && targetPath == "" {
		return nil, utils.Errorf("either --program or --target must be specified")
	} else {
		config.programName = programName
		config.targetPath = targetPath
	}

	// result  writer
	// var writer io.Writer
	outputFile := c.String("output")
	if outputFile == "" {
		log.Infof("output file is not specified, use stdout")
		// writer = os.Stdout
		config.OutputWriter = os.Stdout
	} else {

		if config.Format == "sarif" {
			if filepath.Ext(outputFile) != ".sarif" {
				outputFile += ".sarif"
			}
			if utils.GetFirstExistedFile(outputFile) != "" {
				backup := outputFile + ".bak"
				os.Rename(outputFile, backup)
				os.RemoveAll(outputFile)
			}
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
		_, prog, err := coreplugin.ParseProjectWithAutoDetective(ctx, config.targetPath, "")
		return prog, err
	}
	return nil, utils.Errorf("get program by parameter fail, please check your command")
}

func scan(ctx context.Context, progName string, ruleFilter *ypb.SyntaxFlowRuleFilter) (id string, e error) {
	log.Infof("================= start code scan ================")
	defer func() {
		log.Infof("syntaxflow scan done")
		if err := recover(); err != nil {
			log.Errorf("syntaxflow scan failed: %s", err)
			utils.PrintCurrentGoroutineRuntimeStack()
			e = utils.Errorf("syntaxflow scan failed: %s", err)
		}
	}()
	// start code scan
	var taskId string
	yakgrpc.SyntaxFlowScan(ctx, &ypb.SyntaxFlowScanRequest{
		ControlMode:    "start",
		Filter:         ruleFilter,
		ProgramName:    []string{progName},
		IgnoreLanguage: true,
	}, func(res *ypb.SyntaxFlowScanResponse) error {
		taskId = res.GetTaskID()
		return nil
	})
	return taskId, nil
}

// ShowResult displays scan results based on the provided configuration
// TODO: should use `showRisk` not result
func ShowResult(filter *ypb.SyntaxFlowResultFilter, writer io.Writer) {
	log.Infof("================= show result ================")
	defer func() {
		log.Infof("show sarif result done")
		if err := recover(); err != nil {
			log.Errorf("show sarif result failed: %s", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	showSarifResult(filter, writer)
}

func showSarifResult(filter *ypb.SyntaxFlowResultFilter, writer io.Writer) {
	db := yakit.FilterSyntaxFlowResult(ssadb.GetDB(), filter)

	total, err := ssaapi.CountSyntaxFlowResult(db)
	if err != nil {
		log.Errorf("count syntax flow result failed: %s", err)
		return
	}
	log.Infof("total syntax flow result have risk: %d", total)

	results := ssaapi.YieldSyntaxFlowResult(db)
	report, err := sarif.New(sarif.Version210, false)
	if err != nil {
		log.Errorf("create sarif.New Report failed: %s", err)
		return
	}

	count := 0
	for result := range results {
		count++
		log.Infof("cover result[%d] to sarif run %d/%d: ", result.GetResultID(), count, total)
		run := ssaapi.ConvertSyntaxFlowResultToSarifRun(result)
		if !funk.IsEmpty(run) {
			log.Infof("cover result[%d] add run to report %d/%d done", result.GetResultID(), count, total)
			report.AddRun(run)
		}
	}
	log.Infof("write report ... ")
	report.PrettyWrite(writer)
	log.Infof("write report done")
}
