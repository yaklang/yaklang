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
	"github.com/yaklang/yaklang/common/yak/ssa/ssaprofile"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func SyncEmbedRule(force ...bool) {
	sync := false
	if len(force) > 0 {
		sync = force[0]
	}
	log.Infof("================= check builtin rule sync ================")
	if sync || sfbuildin.CheckEmbedRule() {
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
		}
		_, prog, err := coreplugin.ParseProjectWithAutoDetective(ctx, config.targetPath, config.language, para)
		return prog, err
	}
	return nil, utils.Errorf("get program by parameter fail, please check your command")
}

func scan(ctx context.Context, progName string, ruleFilter *ypb.SyntaxFlowRuleFilter, memory bool) (ch chan *ssaapi.SyntaxFlowResult, e error) {
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
	ch = make(chan *ssaapi.SyntaxFlowResult, 10)
	go func() {
		defer close(ch)
		err := syntaxflow_scan.StartScan(ctx, func(result *syntaxflow_scan.ScanResult) error {
			// 处理扫描结果
			if result.Result == nil {
				return nil
			}

			id := result.Result.ResultID
			kind := result.Result.SaveKind

			// 从缓存中创建结果
			ssaResult := ssaapi.CreateResultFromCache(ssaapi.ResultSaveKind(kind), id)
			if ssaResult == nil {
				return nil
			}

			if ssaResult.RiskCount() > 0 {
				ch <- ssaResult
			} else {
				log.Infof("no risk skip ")
			}
			return nil
		},
			syntaxflow_scan.WithProgramNames(progName),
			syntaxflow_scan.WithRuleFilter(ruleFilter),
			syntaxflow_scan.WithMemory(memory),
		)

		if err != nil {
			log.Errorf("scan failed: %v", err)
		}
	}()
	return ch, nil
}

// ShowRisk displays scan results based on the provided configuration
// TODO: should use `showRisk` not result
func ShowRisk(format sfreport.ReportType, ch chan *ssaapi.SyntaxFlowResult, writer io.Writer, opt ...sfreport.Option) {
	log.Infof("================= show result ================")
	defer func() {
		log.Infof("show sarif result done")
		if err := recover(); err != nil {
			log.Errorf("show sarif result failed: %s", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	// convert result to report
	reportInstance, err := sfreport.ConvertSyntaxFlowResultToReport(format, opt...)
	if err != nil {
		log.Errorf("convert syntax flow result to report failed: %s", err)
		return
	}

	count := 0
	for result := range ch {
		count++
		log.Infof("cover result[%d] to sarif run %d: ", result.GetResultID(), count)
		f1 := func() {
			reportInstance.AddSyntaxFlowResult(result)
		}
		ssaprofile.ProfileAdd(true, "convert result to report", f1)
		log.Infof("cover result[%d] add run to report %d done", result.GetResultID(), count)
	}
	if format == sfreport.IRifyReactReportType {
		if count <= 0 {
			log.Infof("no risk skip save")
			return
		}
		err = reportInstance.Save()
		if err != nil {
			log.Errorf("save report failed: %s", err)
		}
		return
	}
	log.Infof("write report ... ")
	reportInstance.PrettyWrite(writer)
	log.Infof("write report done")
}
